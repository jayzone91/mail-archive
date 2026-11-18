//go:build windows || linux

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/jayzone91/mail-archive/internal/accounts"
	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/backup"
	"github.com/jayzone91/mail-archive/internal/database"
	"github.com/jayzone91/mail-archive/internal/platform"
	"github.com/jayzone91/mail-archive/internal/restore"
	"github.com/jayzone91/mail-archive/internal/snapshot"
	"github.com/jayzone91/mail-archive/internal/web"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("MailArchive beendet", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	data := flag.String("data-dir", "", "Absolutes Verzeichnis für Anwendungsdaten")
	port := flag.Int("port", 8080, "Port auf 127.0.0.1 (0 wählt einen freien Port)")
	packagePath := flag.String("import-package", "", "Verschlüsseltes Sicherungspaket in ein neues Datenverzeichnis importieren")
	snapshotPath := flag.String("snapshot-dir", "", "Verzeichnis des zum Sicherungspaket gehörenden EML-Snapshots")
	flag.Parse()
	if *port < 0 || *port > 65535 {
		return errors.New("ungültiger Port")
	}
	ctx, stop := signal.NotifyContext(context.Background(), platform.StopSignals...)
	defer stop()

	var err error
	if *data == "" {
		*data, err = platform.DataDir()
		if err != nil {
			return err
		}
	}
	if !filepath.IsAbs(*data) {
		return errors.New("das Datenverzeichnis muss absolut sein")
	}
	if *packagePath != "" || *snapshotPath != "" {
		if *packagePath == "" || *snapshotPath == "" {
			return errors.New("-import-package und -snapshot-dir gemeinsam angeben")
		}
		return importInstallation(ctx, *packagePath, *snapshotPath, *data)
	}
	if err := platform.PrivateDir(*data); err != nil {
		return fmt.Errorf("Datenverzeichnis: %w", err)
	}
	lock, err := platform.LockData(filepath.Join(*data, "instance.lock"))
	if err != nil {
		return fmt.Errorf("Datenverzeichnis sperren: %w", err)
	}
	defer lock.Close()
	db, err := database.Open(ctx, filepath.Join(*data, "mailarchive.sqlite"))
	if err != nil {
		return err
	}
	defer db.Close()

	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return err
	}
	defer listener.Close()
	archiveLocation, err := archive.OpenLocation(ctx, db, *data)
	if err != nil {
		return err
	}
	site, err := web.New(ctx, db, *data, listener.Addr().String(), log, archiveLocation)
	if err != nil {
		return err
	}
	workerContext, cancelWorker := context.WithCancel(ctx)
	var workers sync.WaitGroup
	sourceArchive := archiveLocation.Store()
	worker := backup.Service{
		DB: db, Accounts: accounts.Store{DB: db, Vault: platform.NewCredentials(*data)},
		Archive: sourceArchive,
	}
	workers.Go(func() {
		worker.Work(workerContext, log)
	})
	restoreWorker := restore.Service{DB: db, Accounts: worker.Accounts, Archive: sourceArchive}
	workers.Go(func() {
		restoreWorker.Work(workerContext, log)
	})
	workers.Go(func() {
		snapshot.Work(workerContext, db, sourceArchive, *data, log)
	})
	workers.Go(func() {
		archiveLocation.Work(workerContext, log)
	})
	workers.Go(func() {
		sourceArchive.Index(workerContext, db, log)
	})
	defer func() {
		cancelWorker()
		workers.Wait()
	}()
	server := &http.Server{
		Handler: site, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	log.Info("MailArchive gestartet", "url", "http://"+listener.Addr().String())
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func importInstallation(ctx context.Context, packagePath, snapshotPath, dataDir string) error {
	if _, err := os.Lstat(dataDir); err == nil {
		return snapshot.ErrExistingInstallation
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.Open(packagePath)
	if err != nil {
		return err
	}
	defer file.Close()
	fmt.Fprint(os.Stderr, "Sicherungspasswort: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fmt.Errorf("Sicherungspasswort im Terminal lesen: %w", err)
	}
	defer clear(password)
	if err := snapshot.ImportPortable(ctx, file, string(password), snapshotPath, dataDir); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "Sicherungspaket und Archiv verifiziert und importiert. Anwendungsdaten:", dataDir)
	return nil
}

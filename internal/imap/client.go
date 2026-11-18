package imap

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"time"

	protocol "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

type Endpoint struct {
	Host, Username, TLSMode string
	Port                    int
}

func Connect(ctx context.Context, endpoint Endpoint, password []byte) (*imapclient.Client, func(), error) {
	dialer := net.Dialer{Timeout: 8 * time.Second}
	socket, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port)))
	if err != nil {
		return nil, nil, errors.New("IMAP-Server nicht erreichbar.")
	}
	stop := context.AfterFunc(ctx, func() { socket.Close() })
	closeSocket := func() { stop(); socket.Close() }
	config := &tls.Config{ServerName: endpoint.Host, MinVersion: tls.VersionTLS12}
	var client *imapclient.Client
	switch endpoint.TLSMode {
	case "implicit":
		conn := tls.Client(socket, config)
		if err = conn.HandshakeContext(ctx); err == nil {
			client = imapclient.New(conn, nil)
		}
	case "starttls":
		client, err = imapclient.NewStartTLS(socket, &imapclient.Options{TLSConfig: config})
	default:
		err = errors.New("invalid TLS mode")
	}
	if err != nil {
		closeSocket()
		return nil, nil, errors.New("IMAP-TLS-Verbindung fehlgeschlagen. Server, Port und Zertifikat prüfen.")
	}
	closeClient := func() { closeSocket(); client.Close() }
	if err := client.Login(endpoint.Username, string(password)).Wait(); err != nil {
		closeClient()
		return nil, nil, errors.New("IMAP-Anmeldung fehlgeschlagen. Zugangsdaten und Server prüfen.")
	}
	return client, closeClient, nil
}

func Discover(ctx context.Context, endpoint Endpoint, password []byte) ([]*protocol.ListData, error) {
	client, closeClient, err := Connect(ctx, endpoint, password)
	if err != nil {
		return nil, err
	}
	defer closeClient()
	folders, err := client.List("", "*", nil).Collect()
	if err != nil {
		return nil, errors.New("IMAP-Ordner konnten nicht geladen werden.")
	}
	return folders, nil
}

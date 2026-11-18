# MailArchive

Lokales E-Mail-Archiv für Windows und Linux. Der Projektumfang und die Entwicklungsphasen stehen in [PLAN.md](PLAN.md).

## Entwicklung

Die Versionen von Go und Bun stehen in `mise.toml`. Zusätzlich wird `just` benötigt. Mit `mise exec -- just dev` die Weboberfläche starten. `mise exec -- just build-all` erstellt die Windows- und Linux-Binärdateien in `dist/`. Die Binärdateien enthalten die Weboberfläche und benötigen zur Laufzeit nur das jeweilige Zielsystem.

```text
mise exec -- just format
mise exec -- just typecheck
mise exec -- just lint
mise exec -- just test
mise exec -- just build-all
mise exec -- just smoke
```

## Prüfstand der ersten Phase

Formatierung, TypeScript-Prüfung, `go vet` und Go-Tests liefen unter Windows erfolgreich. Die Go-Tests prüfen die Einrichtung und Anmeldung, den Schutz vor fremden Anfrageursprüngen, die Speicherung der Einstellungen über einen Neustart hinweg und die exklusive Sperre des Datenverzeichnisses. Der native Windows-Starttest prüft die Anmeldeseite und das eingebettete Stylesheet. Die Produktionsbuilds für `windows/amd64` und `linux/amd64` waren erfolgreich. Die GitHub-Actions-Matrix führt den Starttest und die Go-Tests nativ auf beiden Plattformen aus. Der Linux-Lauf steht noch aus.

## Erster Start

MailArchive lauscht auf `http://127.0.0.1:8080`. Das Datenverzeichnis liegt unter Windows in `%LOCALAPPDATA%\MailArchive` und unter Linux in `$XDG_DATA_HOME/mailarchive` beziehungsweise `~/.local/share/mailarchive`. `-data-dir` akzeptiert ein dediziertes absolutes Datenverzeichnis und `-port` einen anderen lokalen Port.

Die Anwendung legt für die erste Anmeldung die Datei `setup-token` im Datenverzeichnis an. Den Code im Browser zusammen mit dem gewünschten Administrationspasswort eingeben. Nach erfolgreicher Einrichtung entfernt MailArchive die Datei. Die SQLite-Datenbank enthält den Passwort-Hash und die gespeicherten Einstellungen. Arbeitssitzungen enden beim Neustart.

## Archivspeicher

Die EML-Dateien liegen standardmäßig im Unterordner `archive` des Datenverzeichnisses, unter Windows also in `%LOCALAPPDATA%\MailArchive\archive`. Unter **Einstellungen → Archivspeicher** lässt sich ein anderes absolutes Verzeichnis mit vorhandenem übergeordneten Ordner auswählen. MailArchive wartet auf laufende Archivjobs, kopiert die EML-Originale und prüft ihre Prüfsummen, bevor es auf den neuen Pfad umschaltet. Der Wechsel läuft im Hintergrund und kann nach einem Neustart weiterlaufen. Die Dateien am alten Ort bleiben erhalten. SQLite, Einstellungen und Zugangsdaten verbleiben im Datenverzeichnis.

## Postfächer, Sicherung und Archiv

Unter **Postfächer** ein IMAP-Konto anlegen, die gespeicherte Verbindung testen und die Ordner abrufen. Für einen Anbieterwechsel das Ziel als eigenes Postfach anlegen. Vollständige oder inkrementelle Sicherungen lassen sich für ein aktives Postfach oder als Sammelauftrag starten. **Sicherungen** zeigt den Jobstatus und die Zahl verarbeiteter Nachrichten. Unterbrochene Aufträge werden beim Neustart fortgesetzt. Fehlgeschlagene Aufträge lassen sich dort erneut einplanen.

Unter Windows schützt DPAPI das IMAP-Passwort mit dem Windows-Ausführungskonto. Für eine Linux-Desktopsitzung benötigt MailArchive einen entsperrbaren Secret Service und `secret-tool`. Der Secret Service speichert den Schlüssel für die in SQLite verschlüsselten IMAP-Passwörter. Bei Start durch systemd kann `LoadCredentialEncrypted=mailarchive.key:…` stattdessen einen dauerhaft gesicherten 32-Byte-Schlüssel bereitstellen. Dieser Schlüssel muss für alle Neustarts gleich bleiben.

**Zweitsicherungen** erstellt einen Snapshot mit SQLite-Abbild, archivierten EML-Dateien und verifiziertem Manifest. Ein vorhandenes Zielverzeichnis auf einem unabhängigen Datenträger oder NAS auswählen. Erkannte gemeinsame lokale Datenträger werden abgewiesen. Bei NAS und anderen nicht zuordenbaren Speichern muss der Betreiber die Unabhängigkeit bestätigen. Der Status „Dateien und Prüfsummen verifiziert“ bezieht sich auf den Snapshot-Inhalt.

Unter **Zweitsicherungen → Automatische Zweitsicherung** lässt sich ein unabhängiges Ziel festlegen. Die App plant einen ersten Snapshot und danach weitere Snapshots nach erfolgreichen IMAP-Sicherungen ein. Mehrere ausstehende Sicherungen können in einem Snapshot zusammengefasst werden. Ist das Ziel vorübergehend nicht verfügbar, versucht die App einen fehlgeschlagenen automatischen Auftrag nach fünf Minuten erneut, sofern kein neuerer Snapshot für dieses Ziel ansteht. Die Abdeckung zeigt pro Postfach, wie viele aktuell archivierte Nachrichten im letzten geprüften Snapshot für das ausgewählte Ziel enthalten waren. Alte Snapshots ohne erfasste Abdeckung werden durch den nächsten erfolgreich geprüften Snapshot ergänzt.

Im **Archiv** lassen sich gesicherte Nachrichten offline durchsuchen und als Text lesen. Für HTML-Nachrichten gibt es eine isolierte Ansicht ohne externe Ressourcen sowie eine Textansicht. Originale EML-Dateien und Anhänge stehen als Downloads bereit. Die Volltextsuche wird im Hintergrund aktualisiert und kann aus den Originaldateien neu aufgebaut werden.

Die lokalen Regressionstests verwenden synthetische Postfächer und Nachrichten. Sie decken unter anderem den UIDVALIDITY-Wechsel, den Erhalt der EML-Originalbytes, die Wiederaufnahme fehlgeschlagener Backup-Aufträge, beschädigte Snapshots, den Neuaufbau des Suchindex und die HTML-Isolation ab.

## Hardwarewechsel

Unter **Zweitsicherungen → Sicherungspaket für Hardwarewechsel** einen verifizierten Snapshot auswählen und ein eigenes Sicherungspasswort setzen. MailArchive exportiert die aktuelle SQLite-Datenbank samt Einstellungen, Postfachkonfigurationen und Adminzugang als verschlüsselte `.age`-Datei. Bei abweichendem EML-Bestand zuerst einen neuen Snapshot erstellen. Für den Umzug die Paketdatei und den vollständigen Ordner des ausgewählten Snapshots auf den Zielrechner übertragen.

Vor dem ersten normalen Start auf dem Zielrechner im Terminal importieren. Das Zielverzeichnis muss neu sein und sein übergeordnetes Verzeichnis bereits existieren. Das Sicherungspasswort wird verdeckt abgefragt. Beispiel für Windows:

```powershell
./dist/windows-amd64/mailarchive.exe -data-dir "D:\MailArchive" -import-package "E:\Sicherung\mailarchive-transfer.age" -snapshot-dir "E:\Sicherung\SNAPSHOT-ID"
./dist/windows-amd64/mailarchive.exe -data-dir "D:\MailArchive"
```

Unter Linux dieselben Optionen mit der Linux-Binärdatei und passenden Pfaden verwenden. Nach erfolgreichem Import mit dem bisherigen Adminpasswort anmelden. IMAP-Passwörter neu hinterlegen und die Verbindungen testen. Automatische Sicherungen und das Zweitsicherungsziel auf dem Zielrechner wieder ausdrücklich einrichten.

## Wiederherstellung und Migration

Im **Archiv** eine Nachricht oder mehrere Nachrichten zur Wiederherstellung auswählen. Unter **Wiederherstellung** lassen sich außerdem ganze Ordner und Postfächer auswählen. Die Simulation zeigt die Anzahl der Nachrichten aus dem lokalen Archiv. Anschließend ein aktives Zielpostfach verwenden und entweder die Originalordner übernehmen oder einen alternativen Zielordner angeben.

MailArchive prüft am Ziel die EML-Originalbytes und auf Wunsch auch das ursprüngliche IMAP-Datum und unterstützte Flags. Die serverseitigen Flags `\Deleted` und `\Recent` werden beim Upload ausgelassen. Wenn nach einem Verbindungsabbruch keine eindeutige Zielzuordnung möglich ist, erscheint die Nachricht im Wiederherstellungsprotokoll. Die Wiederaufnahme prüft das Ziel erneut und verhindert den automatischen erneuten Upload bei unklarem Ergebnis.

Unter **Migration** Quellpostfächer ihren All-Inkl-Zielpostfächern zuordnen. Je Postfach zeigt die Übersicht, wie viele Nachrichten der letzte protokollierte IMAP-Abgleich am Laufende gesehen und gesichert hat, wie viele Archiveinträge im geprüften Snapshot enthalten sind und wie viele der aktuell zur Wiederherstellung vorgesehenen Nachrichten im letzten Zielauftrag verifiziert wurden. Für bereits abgeschlossene Altaufträge ohne gespeicherten Quellabgleich ist eine neue Sicherung erforderlich. Die Übersicht nutzt gespeicherte Prüfergebnisse. Vor dem Domaintransfer den Abgleich mit einem aktuellen Backup, verifizierten Snapshot und Test-Restore wiederholen. Der Domaintransfer selbst erfolgt beim Anbieter.

## Automatisierung und Autostart

Unter **Einstellungen** die automatische Sicherung global freigeben und beim gewünschten Postfach aktivieren. Das Standardintervall beträgt sechs Stunden. Ein Postfach startet mit einer Vollsicherung und erhält danach inkrementelle Sicherungen. Fällige Läufe werden nach einem Neustart zusammengefasst. Fehlgeschlagene automatische Aufträge werden mit wachsendem Abstand erneut versucht. Der konfigurierte Worker-Grenzwert bestimmt, wie viele unterschiedliche Postfächer parallel gesichert werden.

- **Windows:** Den Windows-Build an einem dauerhaften Ort ablegen. `deploy/windows/register-mailarchive.ps1 -Executable <Pfad-zur-EXE>` richtet die Aufgabe für die Anmeldung des aktuellen Benutzers ein. Die Skripte am Installationsort belassen. Die Aufgabe startet die Anwendung ohne sichtbares Terminal und schreibt Logs nach `%LOCALAPPDATA%\MailArchive\logs`. Die Anmeldung muss zum DPAPI-Kontext der eingerichteten Konten passen.
- **Linux-Desktop:** Die Linux-Binärdatei nach `~/.local/bin/mailarchive` und `deploy/linux/mailarchive-user.service` nach `~/.config/systemd/user/` kopieren. Die Unit über `systemctl --user enable --now mailarchive-user.service` aktivieren. Für gespeicherte IMAP-Zugänge muss der Secret Service der Sitzung verfügbar sein.
- **Linux-Systemdienst:** Die mitgelieferte System-Unit nutzt ein eigenes Konto `mailarchive`, den Zustand unter `/var/lib/mailarchive` und ein verschlüsseltes systemd-Credential. Das Dienstkonto, die Binärdatei, das Credential und die Unit administrativ bereitstellen. Für ein separat eingehängtes Zweitsicherungsziel den Pfad über `ReadWritePaths=` in einem Unit-Drop-in freigeben. Logs stehen im Systemjournal.

Unter **Aufbewahrung** zeigt MailArchive anhand des zuletzt synchronisierten Bestands, welche älteren Nachrichten außerhalb der standardmäßigen Restore-Schutzfrist von 30 Tagen liegen. Die Vorschau führt keine IMAP-Kommandos aus.

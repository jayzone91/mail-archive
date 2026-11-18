# Codex Agent – Implementierungsplan MailArchive

## Umsetzungsrahmen

MailArchive ist eine lokale E-Mail-Archivierung mit eigener Weboberfläche. MailStore Server dient als funktionale Referenz. Der erste produktive Einsatz ist der Umzug von rund 15 IMAP-Postfächern von Strato zu All-Inkl. Danach übernimmt MailArchive die laufende Sicherung, Suche und Wiederherstellung. Die Grundversion umfasst auch die Sicherung von Exchange Server und Exchange Online gemäß Abschnitt 20 und wird vor dem ersten produktiven Einsatz vollständig abgenommen. Die späteren Erweiterungen in Abschnitt 21 beginnen erst danach.

Beim Planungsstand auf `main` enthielt das Repository nur `PLAN.md` und `.gitattributes`. `just` ist im PATH verfügbar. Go und Bun sind über mise installiert und wurden außerhalb der Sandbox mit `mise exec -- go version` und `mise exec -- bun --version` geprüft. Go- und Bun-Befehle über `mise exec -- …` mit der vorgesehenen Sandbox-Freigabe ausführen, einschließlich der `just`-Aufgaben, die diese Werkzeuge aufrufen. In Phase 1 die Tool-Versionen im Projekt festlegen und reproduzierbare Produktionsbuilds für Windows und Linux einrichten.

## Fortschritt

Die Checkboxen zeigen den Projektstand. Ein Haken bedeutet, dass die konkret genannte Funktion umgesetzt oder die genannte Prüfung bestanden ist. Eine implementierte Teilfunktion kann abgehakt sein, während die Abnahme der zugehörigen Phase noch offen ist. Nach jeder Umsetzung die betroffenen Punkte hier und in der [Definition of Done](#17-definition-of-done) aktualisieren.

**Nächster Umsetzungsschritt:** Regelmäßige Datenbank- und Archivintegritätsprüfungen ergänzen. Die praktischen Abnahmen sind gesammelt offen.

### IMAP, Archiv und Wiederherstellung

- [x] Go-Anwendung mit SQLite-Migrationen, Admin-Ersteinrichtung und lokaler Oberfläche aufgesetzt.
- [x] Windows- und Linux-Produktionsbuilds erzeugt und Windows-Start lokal geprüft.
- [x] IMAP-Postfachverwaltung mit verschlüsselten Zugangsdaten, TLS-Verbindungstest und Ordnerabruf umgesetzt.
- [x] Vollständige und inkrementelle IMAP-Sicherung mit verifizierten EML-Originalen und wiederaufnehmbaren Aufträgen umgesetzt.
- [x] Praxistest bestanden. Zwei Postfächer mit rund 250 E-Mails gesichert und die Nachrichten im Archiv angezeigt.
- [x] Archivansicht mit Postfachauswahl, Ordnerbaum, Volltextsuche, Text- und HTML-Ansicht sowie Downloads umgesetzt.
- [x] IMAP-Wiederherstellung für Nachrichten, Ordner und ganze Postfächer mit Vorschau, Journal und Zielprüfung umgesetzt.
- [x] Praxistest bestanden. 297 E-Mails in ein anderes Postfach wiederhergestellt und beim erneuten Restore keine Duplikate erzeugt.
- [ ] Leere IMAP-Ordner beim Restore wieder anlegen und den vollständigen Ordnerbaum im Ziel prüfen.

### Zweitsicherung, Hardwarewechsel und Migration

- [x] Manuellen Snapshot mit SQLite-Abbild, EML-Dateien und geprüftem Manifest umgesetzt.
- [x] Migrationsübersicht mit Quell-/Zielzuordnung und Restore-Status umgesetzt.
- [x] Zweitsicherungen nach erfolgreichen Backups automatisch einplanen und die Abdeckung des letzten verifizierten Snapshots je Postfach anzeigen. Folgesnapshot, Wiederholung und Teilabdeckung sind mit synthetischen Daten getestet.
- [x] Verschlüsseltes Paket für SQLite, Einstellungen und Adminzugang in der Admin-Oberfläche exportieren und per Kommandozeile mit dem passenden EML-Snapshot in ein neues Datenverzeichnis importieren.
- [ ] Wiederherstellung aus Paket und passendem EML-Snapshot auf einem Ersatzrechner ohne Zugriff auf das Altsystem abnehmen.
- [x] Je Postfach Abschlussbestand des letzten protokollierten IMAP-Abgleichs, Abdeckung des letzten verifizierten Snapshots und geprüfte Nachrichten des Zielauftrags gegenüberstellen.
- [ ] Strato → All-Inkl mit vollständigem Abschlussabgleich und Test-Restore vor dem produktiven Domaintransfer abnehmen.

### Automatisierung, Wartung und Retention

- [x] Zeitgesteuerte Backups, persistente Wiederholungsversuche, parallele Worker und Live-Status umgesetzt.
- [x] Windows-Aufgabenplanung und Linux-systemd als Startoptionen vorbereitet.
- [x] Aufbewahrungsvorschau anhand lokaler IMAP-Metadaten und der Restore-Schutzfrist umgesetzt.
- [ ] Zeitfenster für Hintergrundaufgaben und regelmäßige Datenbank- und Archivintegritätsprüfungen ergänzen.
- [ ] Löschvorschau gegen den aktuellen Serverbestand und die unabhängige Zweitsicherung verifizieren.
- [ ] Gezielte serverseitige Bereinigung mit `UID EXPUNGE`, Freigabe je Konto und Löschhistorie umsetzen und mit isolierten Postfächern abnehmen.
- [ ] Nativen Linux-Lauf, Autostart auf beiden Plattformen und Wiederaufnahme nach simuliertem Absturz abnehmen.

### Exchange und erster Release

- [ ] Exchange Server On-Premises mit zentralem Admin- oder Dienstkonto und selektiver Postfachsicherung umsetzen und abnehmen.
- [ ] Exchange Online mit zentraler Entra-ID-App und selektiver Postfachsicherung umsetzen und abnehmen.
- [ ] Funktionsabnahme der Grundversion abschließen. Erst danach Installer, Release-Automatisierung und Updates angehen.
- [ ] Windows-Installer mit Admin-Ersteinrichtung und Import für Hardwarewechsel umsetzen und testen.
- [ ] Signierte Releases und verifizierten Update- und Rollback-Ablauf für Windows und Linux abnehmen.

### Späterer Ausbau

- [ ] Archivverschlüsselung, eigenständige und unveränderbare Archivbestände sowie datenschutzgerechte Verwaltung umsetzen.
- [ ] Lokale Benutzerverwaltung mit Lese- und Wiederherstellungsrechten je Archiv umsetzen.
- [ ] Admin-Client mit dem Server ausliefern und separaten Benutzer-Client für Windows und Linux abnehmen.

## 1. Projektauftrag

Entwickle eine vollständige, lokal betriebene E-Mail-Archivierungssoftware namens **MailArchive**.

Die Anwendung soll unter Windows 11 sowie auf aktuellen Linux-Distributionen laufen und mehrere IMAP-Postfächer verwalten. Plattformunabhängige Fachlogik ist von klar abgegrenzten Betriebssystem-Adaptern zu trennen.

### Ausgangssituation

- Aktueller E-Mail-Anbieter: Strato
- Zukünftiger Anbieter: All-Inkl
- Anzahl der Postfächer: ungefähr 15
- Protokoll: IMAP über TLS
- Domaintransfer steht noch aus
- Sämtliche E-Mails müssen vor dem Domaintransfer lokal gesichert werden
- Nach dem Domaintransfer sollen die archivierten E-Mails bei All-Inkl wiederhergestellt werden
- Anschließend soll die Software dauerhaft als lokales E-Mail-Archiv arbeiten

### Langfristiges Ziel

Der Mailserver soll nur die Nachrichten der letzten zwei Monate vorhalten.

Alle älteren Nachrichten werden automatisch archiviert und nach erfolgreicher, verifizierter und zusätzlich gesicherter Archivierung vom Mailserver entfernt.

Über eine lokale Weboberfläche müssen sämtliche archivierten Nachrichten jederzeit lesbar und wiederherstellbar sein.

---

## 2. Anweisungen an den Codex-Agenten

Vor der Implementierung:

1. Das angegebene GitHub-Repository klonen beziehungsweise den vorhandenen Workspace untersuchen.
2. Aktuellen Branch, Git-Status, Verzeichnisstruktur und sämtliche vorhandenen Dateien prüfen.
3. Keine vorhandenen Implementierungen grundlos ersetzen.
4. Keine Repository-Strukturen, Dateien oder Funktionen erfinden.
5. Falls das Repository leer ist, die Projektstruktur neu erstellen.
6. Die Implementierung in klar abgegrenzte, testbare Module aufteilen.
7. Möglichst keine Go-, TypeScript- oder templ-Datei mit mehr als 500 Zeilen erstellen.
8. Keine unnötigen Frameworks oder externen Dienste verwenden.
9. Änderungen vor jedem Commit lokal testen.
10. Nur erfolgreiche und getestete Änderungen committen.
11. Nicht ohne ausdrückliche Berechtigung pushen.

Entwicklungsgrundsätze:

- Saubere Trennung von Backend, Datenhaltung und Weboberfläche.
- Kein Docker.
- Keine Cloud-Abhängigkeit.
- Keine Electron-Anwendung.
- Keine separaten Node.js-Webserver.
- Go liefert Backend und Weboberfläche aus.
- Konfiguration überwiegend über die Weboberfläche.
- Reproduzierbare Builds über ein `justfile`.
- Fehlerbehandlung und strukturierte Logs verpflichtend.
- Kritische Aktionen müssen idempotent und nach einem Absturz wiederaufnehmbar sein.
- Keine echten E-Mails oder Zugangsdaten in Tests verwenden.

---

## 3. Technologiestack

| Bereich                 | Technologie                         |
| ----------------------- | ----------------------------------- |
| Backend                 | Go, aktuelle stabile Version        |
| HTTP-Server             | Go `net/http`                       |
| IMAP                    | `github.com/emersion/go-imap/v2`    |
| Datenbank               | SQLite                              |
| SQLite-Treiber          | `modernc.org/sqlite`                |
| Volltextsuche           | SQLite FTS5                         |
| Templates               | templ                               |
| Frontend                | TypeScript                          |
| Styling                 | SCSS                                |
| Bundler                 | Bun                                 |
| Interaktivität          | htmx, wo sinnvoll                   |
| Build-Automatisierung   | just                                |
| Zugangsdaten            | OS-Adapter: Windows DPAPI, Linux Secret Service beziehungsweise systemd-Credentials für Dienste |
| Hintergrundverarbeitung | Go Worker                           |
| Programmstart           | OS-Adapter: Windows-Aufgabenplanung, Linux systemd |
| E-Mail-Speicherung      | EML, unveränderte RFC-5322-Rohdaten |
| Logging                 | `log/slog`                          |

Alle Bibliotheksversionen vor der Verwendung prüfen und kompatible Versionen festlegen.

---

## 4. Systemarchitektur

Die Anwendung besteht aus sieben logisch getrennten Komponenten.

### 4.1 Webserver

Stellt die lokale Weboberfläche und die internen API-Endpunkte bereit.

Standardadresse:

`http://127.0.0.1:8080`

Keine externe Netzwerkfreigabe in der Standardkonfiguration.

### 4.2 IMAP-Service

Verantwortlich für:

- Verbindung zu IMAP-Servern
- Authentifizierung
- Ermittlung der Ordnerstruktur
- Ermittlung aller Nachrichten
- Herunterladen vollständiger Nachrichten
- Auslesen von UID, UIDVALIDITY und Flags
- Hochladen und Wiederherstellen
- Kontrolliertes Löschen
- Verbindungsabbrüche, Timeouts und Wiederholungen

### 4.3 Archive-Service

Verantwortlich für:

- Speicherung vollständiger EML-Dateien
- Integritätsprüfung über SHA-256
- Persistente Nachrichtenmetadaten
- Wiederaufnahme unterbrochener Backups
- Vermeidung von Duplikaten
- Archivindexierung
- Verwaltung archivierter Nachrichten

### 4.4 Backup-Service

Organisiert vollständige und inkrementelle Sicherungen.

Postfächer werden unabhängig voneinander verarbeitet.

Ein fehlgeschlagenes Postfach darf nicht die Sicherungen aller anderen Postfächer verhindern.

### 4.5 Retention-Service

Verwaltet die automatische Bereinigung von IMAP-Servern.

Standardmäßig werden Nachrichten entfernt, deren IMAP-INTERNALDATE älter als zwei Kalendermonate ist.

Die Bereinigung darf niemals Nachrichten entfernen, deren Archivierung oder zusätzliche Sicherung nicht erfolgreich verifiziert wurde.

### 4.6 Restore-Service

Ermöglicht das Zurückspielen von Nachrichten:

- Einzelne E-Mail
- Mehrere ausgewählte E-Mails
- Ganzer Ordner
- Vollständiges Postfach
- Wiederherstellung auf einem anderen IMAP-Server

### 4.7 Scheduler

Verarbeitet wiederkehrende Aufgaben innerhalb des laufenden Go-Prozesses.

Aufgaben müssen nach Neustarts kontrolliert fortgeführt werden können.

Ein plattformspezifischer Start-Adapter integriert die Anwendung unter Windows in die Aufgabenplanung und unter Linux in systemd. Der Start erfolgt beim Benutzerlogin beziehungsweise über eine explizit konfigurierte Benutzer- oder Systemdiensteinheit.

Für unbeaufsichtigten Betrieb ohne Benutzeranmeldung muss auf beiden Plattformen eine separate Betriebsart mit geeignetem Dienstkonto und geklärtem Zugriff auf verschlüsselte Zugangsdaten unterstützt werden.

### 4.8 Plattformintegration

Betriebssystemabhängige Funktionen hinter kleinen Go-Interfaces kapseln und über getrennte Implementierungen beziehungsweise Go-Build-Tags bereitstellen. Dazu gehören mindestens:

- Verschlüsselung und Entschlüsselung von Zugangsdaten
- Autostart, Dienstinstallation und Dienststatus
- Ermittlung der Standardverzeichnisse für Konfiguration, Daten, Cache und Logs
- Dateisystemberechtigungen und plattformspezifische Dateisperren
- Signalbehandlung und sauberer Shutdown

Die Fachmodule dürfen Windows- oder Linux-APIs nicht direkt aufrufen. Nicht unterstützte Plattformen müssen beim Build oder Start mit einer verständlichen Fehlermeldung abgewiesen werden. Windows- und Linux-Adapter erhalten jeweils eigene automatisierte Tests; betriebssystemspezifische Integrationstests laufen auf dem jeweiligen Zielsystem.

---

## 5. Datenbank

SQLite mit WAL-Modus, Foreign Keys und versionierten Migrationen verwenden.

### 5.1 Tabellen

#### settings

Globale Anwendungseinstellungen:

- Archivpfad
- Backupintervall
- Standard-Aufbewahrungsfrist
- Bereinigungsintervall
- Anzahl paralleler Worker
- Maximale Wiederholungsversuche
- Backup-Verifizierung
- Zweitsicherungsziel
- Webserver-Konfiguration
- Zeitstempel der letzten Änderungen

#### accounts

IMAP-Postfächer:

- ID
- Anzeigename
- E-Mail-Adresse
- Benutzername
- Server
- Port
- TLS-Modus
- Verschlüsselte Zugangsdaten
- Aktiviert/Deaktiviert
- Automatisches Backup aktiviert
- Automatische Bereinigung aktiviert
- Individuelle Aufbewahrungsfrist
- Letzte erfolgreiche Synchronisierung
- Letzte Bereinigung
- Letzter Fehler
- Erstellungsdatum
- Änderungsdatum

Eine identische E-Mail-Adresse darf während einer Migration in unterschiedlichen Serverkonfigurationen existieren, ohne die Herkunft archivierter Nachrichten zu verlieren.

#### folders

- ID
- Account-ID
- Original-IMAP-Pfad
- Anzeigename
- Ordnerattribute
- UIDVALIDITY
- UIDNEXT
- Synchronisierungsstatus
- Zeitstempel

#### messages

- Interne Nachrichten-ID
- Ursprünglicher Account
- Ursprünglicher Ordner
- UID
- UIDVALIDITY
- Message-ID
- Absender
- Empfänger
- CC
- Betreff
- Sendedatum
- IMAP-INTERNALDATE
- Flags
- Dateigröße
- SHA-256
- EML-Speicherreferenz
- Archivierungsdatum
- Verifizierungsstatus
- Serverseitig vorhanden
- Serverseitig entfernt
- Zeitpunkt der serverseitigen Löschung

Nachrichtenidentität nicht allein auf der Message-ID oder UID aufbauen.

#### attachments

- ID
- Nachrichten-ID
- Dateiname
- MIME-Type
- Größe
- MIME-Part-Referenz

Anhänge müssen nicht separat doppelt gespeichert werden. Sie können beim Zugriff aus der archivierten Originalnachricht gelesen werden.

#### jobs

- ID
- Job-Typ
- Account-ID
- Status
- Startzeit
- Endzeit
- Fortschritt
- Anzahl verarbeitet
- Anzahl erfolgreich
- Anzahl fehlgeschlagen
- Fehlermeldung
- Wiederholungsversuche

#### restore_history

- ID
- Quellnachrichten
- Ziel-Account
- Zielordner
- Startzeit
- Ergebnis
- Fehler
- Zugeordnete Ziel-UIDs, sofern verfügbar

#### deletion_history

- Account
- Ordner
- UID
- UIDVALIDITY
- Archiv-ID
- Verifizierungsnachweis
- Löschzeitpunkt
- Ergebnis

#### backup_snapshots

- Snapshot-ID
- Erstellungszeit
- Speicherort
- Manifest
- Integritätsstatus
- Anzahl Nachrichten
- Gesamtgröße

#### audit_events

Revisionsprotokoll für:

- Änderung von Zugangsdaten
- Änderungen der Aufbewahrungsfrist
- Bereinigungen
- Wiederherstellungen
- Archivpfadänderungen
- Änderungen sicherheitsrelevanter Einstellungen

### 5.2 Volltextsuche

SQLite FTS5 verwenden.

Indexierbare Felder:

- Absender
- Empfänger
- Betreff
- Textinhalt
- Optional Textinhalte unterstützter Anhänge in späterer Ausbaustufe

HTML-Nachrichten müssen für die Suchindexierung sicher in Text umgewandelt werden.

Suchindex muss bei Bedarf vollständig aus dem Archiv rekonstruiert werden können.

---

## 6. Weboberfläche

Die Oberfläche soll modern, responsiv und übersichtlich aufgebaut werden.

Designvorgaben:

- Sidebar-Navigation
- Dark Mode und Light Mode
- Tabellen mit Filterung und Sortierung
- Fortschrittsanzeigen
- Statusindikatoren
- Bestätigungsdialoge für destruktive Aktionen
- Desktop-first, aber responsiv

### 6.1 Dashboard

Anzeigen:

- Anzahl konfigurierter Postfächer
- Gesamtanzahl archivierter Nachrichten
- Größe des lokalen Archivs
- Letzte erfolgreiche Sicherung
- Nächste geplante Sicherung
- Letzte Bereinigung
- Letzte Fehler
- Freier Speicherplatz
- Zustand der Zweitsicherung
- Laufende Hintergrundaufgaben

Zusätzlich eine Übersicht über alle Postfächer mit ihrem jeweiligen Sicherungsstatus.

### 6.2 Postfachverwaltung

Postfächer über die Weboberfläche anlegen, bearbeiten, aktivieren und deaktivieren.

Eingabefelder:

- Anzeigename
- E-Mail-Adresse
- IMAP-Server
- IMAP-Port
- Benutzername
- Passwort
- TLS-Modus
- Backup aktivieren
- Bereinigung aktivieren
- Individuelle Aufbewahrungsfrist

Funktionen:

- Verbindung testen
- Zugangsdaten aktualisieren
- Ordnerstruktur abrufen
- Backup manuell starten
- Bereinigung manuell starten
- Wiederherstellung starten
- Postfach deaktivieren
- Konfiguration entfernen

Das Entfernen eines Postfachs darf das lokale Archiv niemals automatisch löschen.

Passwörter nach dem Speichern nicht erneut im Klartext an den Browser übertragen.

### 6.3 Archivansicht

Darstellung ähnlich einem klassischen E-Mail-Client.

Linke Spalte:

- Postfachauswahl
- Ordnerbaum

Mittlere Spalte:

- Nachrichtenliste
- Absender
- Betreff
- Datum
- Größe
- Anhänge-Indikator
- Archivstatus

Rechte Spalte beziehungsweise Detailansicht:

- Vollständige Nachricht
- Absender und Empfänger
- CC
- Betreff
- Datum
- HTML-/Textansicht
- Anhänge
- Original-Header bei Bedarf

Funktionen:

- Nachrichten durchsuchen
- Nach Datum filtern
- Nach Absender filtern
- Nach Postfach filtern
- Nachrichten auswählen
- EML herunterladen
- Anhänge herunterladen
- Nachricht wiederherstellen
- Ordner wiederherstellen

HTML-E-Mails ausschließlich isoliert und sicher rendern.

Keine fremden Skripte ausführen.

Externe Ressourcen standardmäßig blockieren, um Tracking und unerwünschte Netzwerkanfragen zu verhindern.

### 6.4 Backupverwaltung

Funktionen:

- Vollständiges Backup starten
- Inkrementelles Backup starten
- Einzelnes Postfach sichern
- Alle Postfächer sichern
- Laufende Sicherungen anzeigen
- Sicherungsfortschritt anzeigen
- Fehler anzeigen
- Abgebrochene Jobs fortsetzen
- Integritätsprüfung starten
- Snapshot erstellen

### 6.5 Wiederherstellung

Ein Wiederherstellungsassistent soll folgende Schritte ermöglichen:

1. Quellpostfach beziehungsweise Archiv auswählen.
2. Nachrichten oder Ordner auswählen.
3. Zielpostfach auswählen.
4. Zielordner bestimmen.
5. Wiederherstellungsoptionen auswählen.
6. Vorschau der geplanten Wiederherstellung anzeigen.
7. Wiederherstellung starten.
8. Fortschritt und Ergebnis anzeigen.

Optionen:

- Originalordner wiederherstellen
- Alternativen Zielordner verwenden
- Ursprüngliches Datum erhalten
- Nachrichtenflags erhalten
- Bereits vorhandene Nachrichten nach Möglichkeit überspringen
- Wiederherstellung nur simulieren

Wiederherstellungen müssen mehrfach ausführbar sein, ohne unkontrolliert Duplikate zu erzeugen.

Dafür Zielserver-Zuordnungen und Restore-Journale verwenden. Bei unklaren Ergebnissen nach Verbindungsabbrüchen vor erneuten Uploads prüfen, ob die Nachricht bereits existiert.

### 6.6 Einstellungen

Sämtliche regulären Anwendungseinstellungen über die Weboberfläche verwalten.

#### Speicher

- Archivverzeichnis
- Zweitsicherungsverzeichnis
- Speicherplatzübersicht
- Integritätsprüfung
- Speicherpfad ändern

Bei Änderung des Speicherpfads vorhandene Archivdateien sicher und überprüfbar migrieren. Währenddessen betroffene Jobs sperren.

#### Automatisierung

- Backupintervall, Standard: 6 Stunden
- Bereinigungsintervall, Standard: 24 Stunden
- Aufbewahrungsfrist, Standard: 2 Kalendermonate
- Automatische Bereinigung aktivieren/deaktivieren
- Zeitfenster für Hintergrundaufgaben
- Parallele Postfachverarbeitung
- Wiederholungsversuche

#### Sicherheit

- Verschlüsselungsstatus
- Lokaler Zugriffsschutz
- Administrationspasswort
- Sitzungssicherheit
- Protokollierung

#### Wartung

- Datenbankintegrität prüfen
- Suchindex neu aufbauen
- Archivprüfung starten
- Alte Jobprotokolle bereinigen
- Fehlerdiagnose exportieren

---

## 7. Backup-Engine

### 7.1 Vollständige Sicherung

Ablauf:

1. Verbindung zum IMAP-Server aufbauen.
2. Zugangsdaten prüfen.
3. Sämtliche verfügbaren Ordner ermitteln.
4. Ordnerattribute und UIDVALIDITY erfassen.
5. Alle relevanten Nachrichten-UIDs ermitteln.
6. Vollständige Rohdaten jeder Nachricht herunterladen.
7. EML-Datei temporär schreiben.
8. SHA-256 berechnen.
9. Datei synchronisieren und atomar an den endgültigen Speicherort verschieben.
10. Metadaten und Nachrichtenreferenzen in SQLite speichern.
11. Persistierte EML-Datei erneut prüfen.
12. Nachrichtenstatus aktualisieren.
13. Ergebnis protokollieren.

### 7.2 Inkrementelle Sicherung

Neue Nachrichten anhand von UID und UIDVALIDITY identifizieren.

Bereits archivierte Nachrichten nicht erneut herunterladen, sofern ihre Identität und der persistierte Zustand unverändert sind.

Bei geänderter UIDVALIDITY den Ordner neu abgleichen und vorhandene Inhalte anhand zusätzlicher Merkmale zuordnen.

Keine Nachrichten allein aufgrund einer geänderten UIDVALIDITY löschen oder überschreiben.

IMAP-Flags regelmäßig aktualisieren, solange Nachrichten auf dem Server vorhanden sind. Änderungen historisch nachvollziehbar halten.

### 7.3 Fehlerbehandlung

Folgende Situationen berücksichtigen:

- Verbindungsabbruch
- Timeout
- IMAP-Server nicht erreichbar
- Ungültige Zugangsdaten
- TLS-Fehler
- Speicherplatzmangel
- Beschädigte EML-Datei
- Neustart während eines Downloads
- Datenbankfehler
- Veränderte Ordnerstruktur
- Gleichzeitige Änderungen am Mailserver

Eine teilweise gespeicherte Nachricht darf niemals als erfolgreich archiviert gelten.

### 7.4 Archivformat

Nachrichten als unveränderte EML-Rohdaten speichern.

Zusätzliche IMAP-Metadaten in SQLite speichern.

Keine Transformation der Originalnachricht vornehmen.

Dateinamen und Archivpfade müssen unabhängig von problematischen Nachrichtenheadern sein.

Archivdateien ausschließlich über interne, sichere Identifikatoren adressieren.

---

## 8. Automatische Bereinigung

### 8.1 Grundregel

Alle Nachrichten, deren IMAP-INTERNALDATE älter als zwei Kalendermonate ist, sind Kandidaten für die serverseitige Löschung.

Die Aufbewahrungsfrist muss global und pro Postfach konfigurierbar sein.

### 8.2 Bereinigungsvoraussetzungen

Eine Nachricht darf ausschließlich gelöscht werden, wenn:

1. Die Originalnachricht vollständig archiviert ist.
2. Der SHA-256-Wert geprüft wurde.
3. Die Nachricht in SQLite korrekt referenziert wird.
4. Die EML-Datei tatsächlich lesbar ist.
5. Ein erfolgreich verifizierter unabhängiger Backup-Snapshot die Nachricht enthält.
6. Die Nachricht eindeutig dem aktuellen IMAP-Ordner zugeordnet ist.
7. UIDVALIDITY und UID vor der Löschung nochmals geprüft wurden.
8. Die Aufbewahrungsfrist überschritten ist.
9. Keine laufende Wiederherstellung oder andere gesperrte Operation kollidiert.
10. Der Bereinigungsjob keine kritischen Fehler aufweist.

### 8.3 Ablauf

1. Erfolgreiche Archivierung sicherstellen.
2. Integrität prüfen.
3. Unabhängige Zweitsicherung aktualisieren und verifizieren.
4. Löschkandidaten bestimmen.
5. Kandidaten unmittelbar vor Löschung erneut mit dem IMAP-Server abgleichen.
6. Löschliste protokollieren.
7. Nachrichten gezielt mit `\Deleted` markieren.
8. Ausschließlich die ausgewählten UIDs über UID EXPUNGE entfernen.
9. Ergebnis kontrollieren.
10. Löschhistorie persistieren.

Wenn der Server kein sicheres gezieltes EXPUNGE unterstützt, Bereinigung für das betreffende Postfach blockieren und eine verständliche Fehlermeldung anzeigen.

Kein pauschales EXPUNGE auf möglicherweise fremd markierten Nachrichten ausführen.

### 8.4 Sicherheitsmechanismen

- Automatische Bereinigung standardmäßig bei neuen Accounts deaktiviert.
- Erste Bereinigung nur nach erfolgreicher vollständiger Sicherung und Wiederherstellungsprobe freigeben.
- Dry-Run-Modus.
- Vorschau geplanter Löschungen.
- Optionales tägliches Löschlimit.
- Fehler führen zum Abbruch des betroffenen Bereinigungslaufs.
- Keine automatische Löschung lokaler Archivdateien.
- Keine automatische Löschung bei nicht verfügbarem Zweitbackup.
- Archiv und Zweitbackup dürfen nicht dasselbe physische Speichermedium als einzige Ausfallsicherung nutzen.

Die Bereinigungslogik muss für alle relevanten IMAP-Ordner funktionieren. Ordner können individuell ausgeschlossen werden.

### 8.5 Bereits wiederhergestellte Nachrichten

Wird eine alte archivierte Nachricht erneut auf den Server übertragen, darf die nächste Bereinigung sie nicht unmittelbar wieder entfernen.

Dafür einen konfigurierbaren Wiederherstellungsschutz verwenden.

Standard: 30 Tage ab erfolgreicher Wiederherstellung.

---

## 9. Wiederherstellungs-Engine

### 9.1 IMAP-Wiederherstellung

Originalnachrichten über IMAP APPEND übertragen.

Dabei möglichst erhalten:

- Originalnachricht
- Ursprünglichen Ordner
- IMAP-INTERNALDATE
- Unterstützte Flags
- Anhänge
- MIME-Struktur

Bei nicht unterstützten Flags oder Ordnernamen Probleme einzeln protokollieren.

### 9.2 Zielserver

Jedes konfigurierte Postfach kann grundsätzlich als Ziel verwendet werden.

Quellpostfach und Zielpostfach müssen nicht identisch sein.

Dadurch wird die Migration von Strato nach All-Inkl ermöglicht.

### 9.3 Bulk-Restore

Die Anwendung muss vollständige Postfächer wiederherstellen können.

Anforderungen:

- Fortschrittsanzeige
- Wiederholbare Jobs
- Kontrollierte Parallelität
- Überprüfung des Upload-Ergebnisses
- Duplikatvermeidung
- Fehlerprotokollierung
- Fortsetzung nach Unterbrechung
- Zusammenfassung nach Abschluss

### 9.4 Migration Strato zu All-Inkl

Eigener Migrationsworkflow:

1. Alle Strato-Postfächer konfigurieren.
2. Vollständiges Backup starten.
3. Archiv und Zweitsicherung vollständig prüfen.
4. Wiederherstellung mit einem Testpostfach verifizieren. Unmittelbar vor dem Domaintransfer einen Abschlussabgleich aller Quellpostfächer durchführen und die Zweitsicherung für den aktualisierten Bestand verifizieren.
5. Domaintransfer extern durchführen.
6. Neue All-Inkl-Postfächer konfigurieren.
7. Verbindung testen.
8. Archiv dem jeweiligen Zielpostfach zuordnen.
9. Alle Nachrichten und Ordner wiederherstellen.
10. Wiederhergestellte Nachrichten je Ordner anhand von Anzahl, EML-Prüfsummen, IMAP-INTERNALDATE und unterstützten Flags abgleichen. Identische Nachrichten mit ihrer jeweiligen Anzahl berücksichtigen. Solange Strato erreichbar bleibt, vor der Stilllegung auf Nachzügler prüfen und diese sichern, wiederherstellen und verifizieren.
11. Regelmäßige Backups auf die neuen Postfächer umstellen.
12. Automatische Bereinigung erst nach erfolgreicher Migration aktivieren.

Bestehende Strato-Archive müssen dauerhaft erhalten bleiben.

---

## 10. Sicherheit

### 10.1 Credentials

Alle IMAP-Passwörter verschlüsselt speichern.

Unter Windows die Data Protection API verwenden. Unter Linux im interaktiven Desktopbetrieb den Secret Service über D-Bus verwenden. Für einen systemd-Systemdienst systemd-Credentials oder eine gleichwertige, dokumentierte Linux-native Schlüsselübergabe verwenden; Zugangsdaten niemals unverschlüsselt in Unit-Dateien, Umgebungsvariablen oder SQLite ablegen.

Es darf niemals ein Klartextpasswort in SQLite, Logs oder HTTP-Antworten erscheinen.

Der Verschlüsselungskontext muss zum tatsächlichen Windows- beziehungsweise Linux-Ausführungskonto und zur gewählten Betriebsart passen.

Änderungen des Dienstkontos dürfen nicht unbemerkt zum Verlust des Zugriffs auf gespeicherte Zugangsdaten führen.

### 10.2 TLS

- Zertifikatsprüfung aktiv
- Kein standardmäßiges `InsecureSkipVerify`
- IMAPS beziehungsweise STARTTLS explizit unterstützen
- Unverschlüsselte Authentifizierung standardmäßig untersagen

### 10.3 Webserver

- Standardmäßig nur Loopback
- Lokale Administratorauthentifizierung
- Sichere Sessions
- CSRF-Schutz
- Sicherheitsheader
- Schutz vor Path Traversal
- Keine offenen Datei-Endpunkte
- HTML-E-Mails sicher isoliert rendern
- Externe Inhalte standardmäßig deaktivieren

### 10.4 Archiv

Dateisystemberechtigungen so setzen, dass nur das vorgesehene Ausführungskonto und erforderliche Administratoren Zugriff erhalten. Unter Windows geeignete ACLs, unter Linux restriktive Besitzer-/Gruppenrechte und standardmäßig `0700` für Verzeichnisse beziehungsweise `0600` für sensible Dateien verwenden.

Bei unverschlüsseltem Archiv ausdrücklich berücksichtigen, dass EML-Dateien personenbezogene Daten enthalten.

Optional spätere Archivverschlüsselung vorbereiten, aber nicht zulasten einer funktionierenden Migration vorziehen.

---

## 11. Archiv-Zweitsicherung

Das lokale Archiv ist die primäre historische Datenquelle. Deshalb muss zusätzlich eine unabhängige Sicherung existieren.

### Anforderungen

- Separates konfigurierbares Ziel
- Geeignet für zweite SSD, externe Festplatte oder NAS
- Konsistentes SQLite-Backup über die SQLite-Backup-API beziehungsweise geeigneten Snapshot-Mechanismus
- Sicherung der vollständigen EML-Dateien
- Manifest mit Dateigrößen und Prüfsummen
- Integritätsprüfung nach der Sicherung
- Protokollierung
- Wiederaufnahme unterbrochener Übertragungen
- Versionierte Snapshots beziehungsweise Schutz gegen unbeabsichtigtes Überschreiben oder Löschen

Eine bloße zweite Kopie auf demselben Datenträger genügt nicht.

Die serverseitige Bereinigung muss auf eine bereits erfolgreich verifizierte zweite Kopie der jeweiligen Nachricht angewiesen sein.

### 11.1 Sicherung und Wiederherstellung für einen Hardwarewechsel

In der Administrationsoberfläche ein portables Sicherungspaket erstellen, das ein konsistentes SQLite-Abbild, sämtliche Anwendungseinstellungen, Postfachkonfigurationen und das Administratorkonto einschließlich Passwort-Hash enthält. Sobald die Benutzerverwaltung verfügbar ist, gehören auch Benutzerkonten und ihre Postfachberechtigungen dazu. Das Paket mit einem vom Administrator vergebenen Sicherungspasswort verschlüsseln und bei der Erstellung auf Integrität prüfen. Laufende Anmeldesitzungen und Einmalcodes werden bei einem Umzug verworfen.

Für einen vollständigen Umzug das Paket mit einer verifizierten Zweitsicherung der EML-Dateien desselben Datenstands wiederherstellen. Das Paket muss den benötigten Archivbestand eindeutig ausweisen. Auf dem Zielrechner Archiv- und Zweitsicherungspfade neu zuordnen sowie die Lesbarkeit und Prüfsummen prüfen, bevor automatisierte Jobs wieder freigegeben werden.

Die Wiederherstellung muss auf einer frischen Installation vor der Anlage eines neuen Administratorkontos möglich sein. Nach erfolgreichem Import kann sich der vorhandene Administrator mit seinem bisherigen Passwort anmelden. An das alte Gerät oder Ausführungskonto gebundene IMAP-Zugangsdaten als neu einzurichten kennzeichnen. Den Status je Postfach anzeigen und die Verbindung nach Neueingabe der Zugangsdaten prüfen. Ein fehlgeschlagener Import darf eine bestehende Installation nicht überschreiben.

**Abnahme:** Auf einem Ersatzrechner ohne Zugriff auf das alte System das Sicherungspaket und den zugehörigen EML-Bestand einspielen, sich als bisheriger Administrator anmelden, Einstellungen und Postfachzuordnungen prüfen und archivierte Nachrichten öffnen. Zusätzlich prüfen, dass ein falsches Sicherungspasswort oder ein beschädigtes Paket keine vorhandenen Daten verändert.

---

## 12. Scheduler und Betriebssystem-Integration

### Scheduler

Standardwerte:

| Einstellung              | Standard                         |
| ------------------------ | -------------------------------- |
| Backupintervall          | 6 Stunden                        |
| Bereinigung              | Alle 24 Stunden                  |
| Aufbewahrung             | 2 Kalendermonate                 |
| Wiederholungsversuche    | 3                                |
| Parallele IMAP-Worker    | 2                                |
| Archivintegritätsprüfung | Täglich                          |
| Zweitsicherung           | Nach erfolgreichen Backup-Läufen |

Keine überlappenden Backup-, Restore- oder Bereinigungsoperationen auf demselben Postfach zulassen, wenn diese den Datenzustand beeinflussen können.

### Windows

Die Anwendung muss ohne Terminalfenster dauerhaft ausführbar sein.

Folgende Optionen bereitstellen:

- Manueller Start
- Automatischer Start beim Windows-Login
- Optional unbeaufsichtigter Betrieb über Aufgabenplanung oder Windows-Dienst
- Sauberer Shutdown
- Fortsetzung unterbrochener Jobs

Keine Administratorrechte für den normalen Betrieb voraussetzen.

Für die Installation systemweiter Autostart-Mechanismen können erhöhte Berechtigungen erforderlich sein.

### Linux

Die Anwendung muss ohne offene Shell dauerhaft ausführbar sein.

Folgende Optionen bereitstellen:

- Manueller Start
- Automatischer Start über eine systemd-User-Unit
- Optional unbeaufsichtigter Betrieb über eine gehärtete systemd-System-Unit und ein eigenes Dienstkonto
- Sauberer Shutdown über `SIGTERM` und `SIGINT`
- Fortsetzung unterbrochener Jobs

Die Unit-Dateien dürfen keine Zugangsdaten enthalten. Schreibzugriffe auf Archiv-, Daten- und Logverzeichnisse müssen explizit freigegeben sein. Für eine systemweite Installation können Root-Rechte erforderlich sein; der laufende Prozess selbst darf nicht als Root betrieben werden.

---

## 13. Projektstruktur

Der Agent soll die tatsächliche Repository-Struktur zuerst prüfen.

Bei einem leeren Repository folgende logische Struktur als Ausgangspunkt verwenden:

```text
mailarchive/
├── cmd/
│   └── mailarchive/
├── internal/
│   ├── app/
│   ├── config/
│   ├── database/
│   │   └── migrations/
│   ├── accounts/
│   ├── imap/
│   ├── archive/
│   ├── backup/
│   ├── restore/
│   ├── retention/
│   ├── scheduler/
│   ├── platform/
│   │   ├── windows/
│   │   └── linux/
│   ├── security/
│   ├── search/
│   ├── jobs/
│   └── web/
│       ├── handlers/
│       ├── views/
│       └── middleware/
├── frontend/
│   ├── ts/
│   └── scss/
├── static/
├── tests/
├── docs/
├── go.mod
├── justfile
├── package.json
├── tsconfig.json
├── .gitignore
└── README.md
```

Diese Struktur ist eine Zielvorgabe, keine Behauptung über bereits existierende Dateien.

Keine unnötigen Verzeichnisse oder Platzhalterdateien erstellen.

---

## 14. Buildsystem

Ein `justfile` erstellen.

Erforderliche Aufgaben:

| Befehl           | Funktion               |
| ---------------- | ---------------------- |
| `just dev`       | Entwicklungsmodus      |
| `just build`     | Produktionsbuild       |
| `just test`      | Sämtliche Tests        |
| `just lint`      | Statische Prüfungen    |
| `just typecheck` | TypeScript prüfen      |
| `just format`    | Code formatieren       |
| `just clean`     | Builddateien entfernen |
| `just run`       | Anwendung starten      |

Zusätzliche plattformspezifische Aufgaben:

| Befehl               | Funktion                                       |
| -------------------- | ---------------------------------------------- |
| `just build-windows` | Produktionsbuild für Windows (`windows/amd64`) |
| `just build-linux`   | Produktionsbuild für Linux (`linux/amd64`)     |
| `just build-all`     | Beide Produktionsbuilds erstellen             |
| `just run-windows`   | Windows-Build auf Windows starten              |
| `just run-linux`     | Linux-Build auf Linux starten                  |

`just build` erstellt standardmäßig den Build für das aktuelle Hostsystem. Die Zielrezepte setzen `GOOS` und `GOARCH` explizit, schreiben in getrennte Verzeichnisse wie `dist/windows-amd64/` und `dist/linux-amd64/` und verwenden passende Dateinamen (`mailarchive.exe` beziehungsweise `mailarchive`). Frontend- und templ-Generierung müssen für beide Ziele identisch und nur einmal pro Buildlauf ausführbar sein. Plattformabhängige Go-Dateien über die Suffixe `_windows.go` und `_linux.go` oder präzise Build-Tags auswählen. Ein Cross-Build darf keine Zielsystem-Kommandos ausführen; plattformspezifische Integrationstests laufen nativ oder in einer CI-Matrix auf Windows und Linux.

Frontend über Bun bündeln.

templ-Templates im Build generieren.

Produktionsausgabe möglichst als einzelne Go-Binärdatei mit eingebetteten Frontend-Assets: EXE unter Windows und ausführbare ELF-Datei unter Linux. Plattformspezifische Unit- beziehungsweise Installationsdateien separat mit ausliefern.

Laufzeitdaten und Archive außerhalb des Git-Repositories speichern.

---

## 15. Entwicklungsphasen

### Phase 1 – Grundgerüst

Implementieren:

- Go-Projekt
- SQLite-Datenbank
- Migrationen
- Webserver
- templ-Integration
- Bun-Build
- SCSS
- Logging
- Grundlayout
- Einstellungen
- Grundlegende Sicherheitsarchitektur

**Abnahme:** Anwendung startet unter Windows und Linux und die Weboberfläche ist auf beiden Plattformen erreichbar.

### Phase 2 – Postfachverwaltung

Implementieren:

- Postfächer hinzufügen
- Postfächer bearbeiten
- Postfächer deaktivieren
- Credentials verschlüsseln
- IMAP-Verbindung testen
- Ordnerstruktur lesen
- Verbindungsstatus anzeigen

**Abnahme:** Ein Strato-Testpostfach kann vollständig über das Webinterface konfiguriert und erfolgreich verbunden werden.

### Phase 3 – Backup-Engine

Implementieren:

- Vollständiges Backup
- Inkrementelles Backup
- EML-Speicherung
- SHA-256-Integritätsprüfung
- Metadatenverwaltung
- UID-/UIDVALIDITY-Abgleich
- Wiederaufnahme abgebrochener Jobs
- Fortschrittsanzeige

**Abnahme:** Ein vollständiges Testpostfach inklusive aller Ordner und Nachrichten wird lokal und überprüfbar archiviert.

### Zwischenabnahme nach Phase 3 – Unabhängige Zweitsicherung

Die Zweitsicherung aus Abschnitt 11 bereits hier implementieren. Sie umfasst konsistente SQLite-Snapshots, vollständige EML-Dateien, versionierte Manifeste und Prüfung am unabhängigen Speicherziel. Vor der Migrationsabnahme einen Snapshot auf einer separaten Instanz zurückspielen und sämtliche referenzierten Dateien prüfen.

### Phase 4 – Archiv-Weboberfläche

Implementieren:

- Postfachnavigation
- Ordneransicht
- Nachrichtenliste
- Nachrichtenanzeige
- HTML-/Textdarstellung
- Anhänge
- EML-Download
- Volltextsuche
- Filterung

**Abnahme:** Archivierte Nachrichten können ohne Verbindung zum IMAP-Server vollständig gelesen werden.

### Phase 5 – Wiederherstellung

Implementieren:

- Einzelne E-Mails wiederherstellen
- Mehrfachauswahl
- Ganze Ordner wiederherstellen
- Ganze Postfächer wiederherstellen
- Anderen Zielserver auswählen
- Fortschrittsanzeige
- Restore-Protokollierung
- Duplikatkontrolle

**Abnahme:** Ein archiviertes Testpostfach wird auf einen unabhängigen IMAP-Testserver zurückgespielt und erfolgreich verglichen.

### Phase 6 – Migration Strato zu All-Inkl

Implementieren:

- Übersicht aller Migrationspostfächer
- Zuordnung Quelle/Ziel
- Vollständigkeitsprüfung
- Migrationsjobs
- Ergebnisprüfung
- Protokolle
- Wiederaufnahme bei Fehlern

**Abnahme:** Migration kann aus einem vorhandenen lokalen Archiv ohne Zugriff auf Strato vollständig erfolgen.

### Phase 7 – Automatisierung

Implementieren:

- Scheduler
- Wiederkehrende Backups
- Jobverwaltung
- Fehlerbehandlung
- Retry-Mechanismus
- Windows-Autostart und Linux-systemd-Integration
- Persistente Jobzustände

**Abnahme:** Backups laufen selbstständig und werden nach einem Neustart wieder aufgenommen.

### Phase 8 – Retention mit verifizierter Zweitsicherung

Die nach Phase 3 abgenommene Zweitsicherung in den Löschpfad integrieren. Implementieren:

- Nachweis der unabhängigen Zweitsicherung anhand des Snapshot-Manifests und erneute Integritätskontrolle vor der Löschung
- Retention-Regeln
- Löschvorschau
- Dry-Run
- Gezielt sichere IMAP-Löschung
- Löschhistorie
- Restore-Schutzfrist

**Abnahme:** Ausschließlich eindeutig identifizierte, überprüfte und unabhängig gesicherte Nachrichten werden serverseitig gelöscht.

### Zwischenabnahme vor Phase 9 – Exchange-Sicherung

Die Sicherung von Exchange On-Premises und Exchange Online gemäß Abschnitt 20 implementieren und mit einer separaten Exchange-Testinstallation sowie einem Microsoft-365-Testmandanten abnehmen. Erst danach Installer, Release-Automatisierung und Updates angehen.

### Phase 9 – Produktionsreife

Implementieren und prüfen:

- Windows- und Linux-Build
- Startverhalten
- Ausfallsicherheit
- Große Postfächer
- Speicherplatzmanagement
- Fehlerprotokollierung
- Datenbankintegrität
- Dokumentation
- Installationsanleitung
- Backup- und Restore-Notfallverfahren
- Release- und Updatekonzept aus Abschnitt 19

**Abnahme:** Anwendung arbeitet unter Windows und Linux stabil und kann nach simuliertem Prozess- beziehungsweise Systemabsturz ohne Archivverlust fortfahren. Ein signierter Test-Release kann installiert, aktualisiert und bei einem fehlgeschlagenen Update sicher zurückgerollt werden.

---

## 16. Testanforderungen

Für alle zentralen Module automatisierte Tests schreiben.

### IMAP

- Verbindung erfolgreich
- Zugangsdaten falsch
- TLS-Fehler
- Timeout
- Verbindungsabbruch
- Mehrere Ordner
- Sonderzeichen in Ordnernamen
- UIDVALIDITY-Wechsel
- Geänderte Flags

### Archiv

- Nachricht vollständig speichern
- SHA-256 verifizieren
- Duplikate verhindern
- Defekte Dateien erkennen
- Unterbrochenen Download fortsetzen
- Fehlenden Speicherplatz behandeln
- Archiv erneut indexieren

### Restore

- Einzelne Nachricht
- Vollständiger Ordner
- Vollständiges Postfach
- Originaldatum
- Flags
- Sonderzeichen
- Große Anhänge
- Unterbrechung
- Wiederholung ohne unbeabsichtigte Duplikate

### Bereinigung

Besonders wichtig:

- Keine Löschung ohne erfolgreiches Backup
- Keine Löschung bei beschädigter EML-Datei
- Keine Löschung bei fehlender Zweitsicherung
- Keine Löschung bei UIDVALIDITY-Wechsel
- Keine Löschung bei fehlendem UID EXPUNGE
- Keine Löschung neuer Nachrichten
- Keine Löschung innerhalb der Restore-Schutzfrist
- Keine unbeabsichtigte Löschung fremder Nachrichten
- Korrekte Zeitberechnung an Monatsgrenzen
- Sichere Wiederaufnahme nach Prozessabbruch

Destruktive Tests ausschließlich gegen isolierte Testpostfächer durchführen.

### Updates

- Neue, identische und ältere Version korrekt erkennen
- Vorabversionen nur nach ausdrücklicher Aktivierung anbieten
- Nicht erreichbares GitHub und API-Limits ohne Beeinträchtigung des laufenden Betriebs behandeln
- Manipulierte Artefakte, Signaturen oder Prüfsummen ablehnen
- Falsche Betriebssystem- oder Architekturartefakte ablehnen
- Laufende Jobs vor der Installation kontrolliert beenden oder das Update verschieben
- Datenbankmigration und Neustart erfolgreich durchführen
- Fehlgeschlagenes Update auf die vorherige Binärdatei zurückrollen
- Archiv- und Konfigurationsdaten bei Update und Rollback unverändert erhalten

---

## 17. Definition of Done

Das Projekt gilt als funktional abgeschlossen, wenn:

- [ ] Alle Postfächer ausschließlich über die Weboberfläche verwaltet werden können.
- [ ] Zugangsdaten sicher verschlüsselt gespeichert werden.
- [ ] Alle Ordner und Nachrichten vollständig archiviert werden.
- [ ] Inkrementelle Sicherungen zuverlässig funktionieren.
- [x] Das Archiv unabhängig vom Mailserver lesbar ist.
- [ ] E-Mails inklusive Anhängen angezeigt werden.
- [x] Volltextsuche funktioniert.
- [x] Einzelne Nachrichten wiederhergestellt werden können.
- [x] Ganze Postfächer auf einen anderen IMAP-Server wiederhergestellt werden können.
- [ ] Automatische Sicherungen funktionieren.
- [ ] Automatische Bereinigung nach zwei Monaten funktioniert.
- [ ] Keine Nachricht ohne verifiziertes Archiv und Zweitsicherung gelöscht wird.
- [ ] Archivdaten nach einem Absturz konsistent bleiben.
- [ ] Die Anwendung unter Windows und Linux automatisch startet.
- [ ] Sämtliche Konfigurationsmöglichkeiten über die Weboberfläche erreichbar sind.
- [ ] Alle automatisierten Tests erfolgreich durchlaufen.
- [ ] Die Windows- und Linux-Binärdateien ohne separate Entwicklungswerkzeuge betrieben werden können.
- [x] Windows- und Linux-Produktionsbuilds reproduzierbar über das `justfile` erstellt werden können.
- [ ] Betriebssystemspezifische Funktionen ausschließlich über getestete Plattformadapter angesprochen werden.
- [ ] Freigegebene Versionen als nachvollziehbare GitHub Releases für Windows und Linux veröffentlicht werden können.
- [ ] Die Anwendung Updates sicher erkennen, anbieten, verifizieren, installieren und bei Fehlern zurückrollen kann.

---

## 18. Arbeitsanweisung an Codex

Arbeite die Entwicklungsphasen mit der Zwischenabnahme für die Zweitsicherung nach Phase 3 ab. Die Phasen sind interne Meilensteine. Der produktive Einsatz beginnt erst nach bestandener Gesamtabnahme.

Beginne mit der Prüfung des tatsächlichen GitHub-Repositories.

Erstelle anschließend eine konkrete Implementierungs- und Aufgabenübersicht auf Basis des vorgefundenen Repository-Zustands.

Implementiere nicht sämtliche Phasen ungeprüft in einem einzigen Schritt.

Baue zuerst eine funktionierende Grundlage, dann die Postfachverwaltung und anschließend die Backup-Engine.

Die zuverlässige lokale Sicherung der vorhandenen Strato-Postfächer hat die höchste Priorität, da der Domaintransfer bevorsteht.

Die Webansicht und Archivverwaltung folgen danach.

Vor der Implementierung der automatischen Löschfunktion müssen Backup, Zweitsicherung, Integritätsprüfung und Wiederherstellung nachweislich funktionieren.

Nach jeder Phase:

1. Code formatieren.
2. TypeScript prüfen.
3. Go-Tests ausführen.
4. Produktionsbuild erstellen.
5. Relevante Integrationstests ausführen.
6. Änderungen und Testergebnisse dokumentieren.
7. Erst danach committen.

Bestehende funktionierende Implementierungen erhalten und Änderungen so klein wie sinnvoll halten.

**Wichtig: Datenverlust durch fehlerhafte Archivierung, Migration oder Bereinigung muss durch die Architektur aktiv verhindert werden.**

---

## 19. GitHub Releases und Anwendungsupdates

Die Entwicklung von Installer, Release-Automatisierung und Update-Funktion beginnt erst, wenn alle Funktionen der Grundversion umgesetzt und funktional abgenommen sind. Dazu gehören der verifizierte Hardwarewechsel, die automatische Zweitsicherung, die sichere serverseitige Bereinigung und die Sicherung von Exchange On-Premises und Exchange Online gemäß Abschnitt 20. Die späteren Erweiterungen sind in Abschnitt 21 separat geplant.

Nach bestandener Gesamtabnahme stabile Versionen als GitHub Releases veröffentlichen. Releases entstehen ausschließlich aus versionierten Git-Tags und enthalten mindestens:

- Windows- und Linux-Artefakte je unterstützter Architektur
- Signiertes Windows-Installationspaket je unterstützter Windows-Architektur gemäß Abschnitt 19.4
- Versionsnummer nach Semantic Versioning
- verständliche Release Notes und Hinweise auf inkompatible Änderungen
- SHA-256-Prüfsummen aller Artefakte
- kryptografisch signiertes Release-Manifest mit Version, Zielplattform, Architektur, Dateigröße, Downloadadresse und Prüfsumme
- Installations-, Update- und gegebenenfalls Migrationshinweise

Das `justfile` um reproduzierbare Aufgaben wie `just release-build`, `just release-checksums` und `just release-verify` ergänzen. Das Veröffentlichen eines GitHub Releases bleibt ein separater, ausdrücklich ausgelöster Schritt und darf nicht durch einen normalen Build erfolgen. Zugangstoken und Signaturschlüssel niemals im Repository oder in Buildartefakten speichern. Vor Veröffentlichung müssen Tests, plattformspezifische Builds und die Verifikation der erzeugten Artefakte erfolgreich sein.

### 19.1 Updateprüfung

Die Anwendung prüft in einem konfigurierbaren Intervall über die offizielle GitHub-Releases-Schnittstelle, ob eine neuere stabile Version verfügbar ist. Die Prüfung muss deaktivierbar sein und darf keine Postfach-, Archiv- oder personenbezogenen Daten übertragen. Vorabversionen werden standardmäßig ignoriert und nur über einen explizit gewählten Updatekanal berücksichtigt.

Fehler bei Netzwerkzugriff, TLS, GitHub-Verfügbarkeit oder API-Limits dürfen den Archivbetrieb nicht beeinträchtigen. Die Weboberfläche zeigt aktuelle Version, verfügbare Version, Veröffentlichungsdatum, Release Notes, Downloadgröße und letzten Prüfstatus an. Zusätzlich muss eine manuelle Updateprüfung möglich sein.

### 19.2 Download und Installation

Updates niemals allein aufgrund einer Versionsangabe installieren. Vor der Installation müssen Release-Manifest, Signatur, Zielplattform, Architektur, Dateigröße und SHA-256-Prüfsumme erfolgreich verifiziert werden. Die Anwendung vertraut ausschließlich einem mitgelieferten öffentlichen Release-Schlüssel; ein über denselben Downloadkanal gelieferter Ersatzschlüssel darf nicht automatisch akzeptiert werden.

Ein verfügbares Update zunächst nur anbieten. Download und Installation erfolgen nach ausdrücklicher Bestätigung eines Administrators. Eine vollständig automatische Installation kann später als standardmäßig deaktivierte Option ergänzt werden. Sicherheitsupdates dürfen deutlich gekennzeichnet, aber nicht ohne die konfigurierte Zustimmung installiert werden.

Der plattformspezifische Update-Adapter übernimmt die Installation:

- Unter Windows die neue Binärdatei beziehungsweise ein signiertes Installationspaket außerhalb des laufenden Prozesses austauschen und den Aufgabenplanungs- oder Dienststart berücksichtigen.
- Unter Linux die neue Binärdatei beziehungsweise ein signiertes Paket mit dem vorgesehenen systemd-Dienst aktualisieren. Systemweite Installationen dürfen nur über einen eng begrenzten privilegierten Installationsschritt erfolgen; der normale Anwendungsprozess bleibt unprivilegiert.

Vor dem Austausch keine neuen Jobs starten, laufende kritische Jobs kontrolliert abschließen oder das Update verschieben, Datenbank und Konfiguration konsistent sichern und anschließend sauber neu starten. Archivdaten niemals in das Programmverzeichnis legen oder im Rahmen eines Updates verändern.

### 19.3 Migration, Rollback und Nachweis

Datenbankmigrationen müssen versioniert, vorwärtskompatibel geplant und vor dem Binärdateiaustausch auf ihre Rollback-Auswirkungen geprüft werden. Vor jedem Update einen konsistenten Sicherungspunkt für Datenbank und Konfiguration erstellen. Die vorherige funktionierende Binärdatei und das verifizierte Release-Manifest bis zur erfolgreichen Abschlussprüfung aufbewahren.

Schlagen Installation, Migration, Neustart oder Health-Check fehl, automatisch auf die vorherige Version zurückrollen, sofern keine irreversible Datenmigration erfolgt ist. Irreversible Migrationen erfordern vorab eine deutlich sichtbare Bestätigung und ein dokumentiertes Wiederherstellungsverfahren. Updateprüfung, Zustimmung, Download, Verifikation, Installation, Neustart und Rollback revisionssicher protokollieren, ohne Zugangsdaten oder andere Geheimnisse zu erfassen.

### 19.4 Windows-Installer und Administratorkonto

Bei einer Neuinstallation fragt der Windows-Installer das Administrationspasswort mit verdeckter Eingabe und Bestätigung ab. Er initialisiert die SQLite-Datenbank im Datenverzeichnis des vorgesehenen Windows-Ausführungskontos, führt die regulären Datenbankmigrationen aus und legt das erste lokale Administratorkonto an. Beim ersten Start ist die Anmeldung direkt mit dem gewählten Passwort möglich.

Die Initialisierung verwendet dieselbe Passwortvalidierung und Passwort-Hashing-Logik wie die Anwendung. Das Passwort darf weder in Klartext gespeichert noch über Kommandozeilenargumente, Installationsprotokolle oder temporäre Dateien weitergegeben werden. Datenbankschema und Adminzugang müssen in einem konsistenten Zustand hinterlassen werden. Updates und Reparaturinstallationen erkennen vorhandene Datenbanken und Administratorkonten und überspringen die Passwortabfrage und Kontoanlage.

Die Release-Abnahme prüft die Neuinstallation mit anschließender Anmeldung, einen Abbruch während der Einrichtung und ein Upgrade, bei dem das bestehende Administratorkonto und Archiv erhalten bleiben.

Bei einer Neuinstallation für einen Hardwarewechsel die Wiederherstellung aus einem vorhandenen Sicherungspaket gemäß Abschnitt 11.1 anbieten. Die Einrichtung eines neuen Administratorkontos erfolgt beim regulären Neuinstallationspfad.

---

## 20. Exchange-Sicherung als Teil der Grundversion

1. **Exchange On-Premises.** Einen Connector für vollständige und inkrementelle Postfachsicherungen in das lokale Archiv entwickeln. Exchange-Administratoren richten einmal pro Organisation einen zentralen Zugang ein. MailArchive authentifiziert sich mit einem entsprechend berechtigten Administratorkonto oder einem vom Administrator eingerichteten Dienstkonto und ermittelt darüber die verfügbaren Postfächer. In der Weboberfläche lassen sich einzelne, mehrere oder alle verfügbaren Postfächer für die Sicherung auswählen. Die Sicherung läuft über den zentralen Zugang ohne Passwörter der Postfachinhaber. Unterstützte Exchange-Versionen, Postfachtypen und die nötigen Berechtigungen für Postfachermittlung und Sicherung in dieser Ausbaustufe festlegen. In einer separaten Exchange-Installation prüfen, dass nur ausgewählte Postfächer archiviert werden und entzogene Rechte als Fehler sichtbar sind. [Microsoft-Dokumentation zu EWS-Impersonation](https://learn.microsoft.com/exchange/client-developer/exchange-web-services/impersonation-and-ews-in-exchange)
2. **Exchange Online und Microsoft 365.** Einen Connector mit zentral registrierter Microsoft-Entra-ID-Anwendung (Azure-App) pro Mandant entwickeln. Ein Mandantenadministrator erteilt einmalig die erforderlichen Anwendungsberechtigungen für die Postfachermittlung und den lesenden Zugriff über Microsoft Graph. MailArchive nutzt OAuth 2.0 Client Credentials für unbeaufsichtigte Voll- und inkrementelle Sicherungen ohne Anmeldung oder OAuth-Freigabe durch einzelne Postfachinhaber. Bei mandantenweiter Berechtigung stehen alle unterstützten Postfächer zur Auswahl. Im Browser lassen sich einzelne, mehrere oder alle verfügbaren Postfächer für die Sicherung auswählen. Die Auswahl steuert den Sicherungsumfang. Eine zusätzliche serverseitige Eingrenzung des App-Zugriffs über Exchange Online Application RBAC soll unterstützt werden. In einem separaten Testmandanten Listenabruf, selektive Sicherung ohne Benutzeranmeldung und die Behandlung fehlender Berechtigungen prüfen. Die archivierten Nachrichten bleiben lokal durchsuchbar. [Microsoft-Dokumentation zu App-Berechtigungen in Exchange Online](https://learn.microsoft.com/en-us/exchange/permissions-exo/application-rbac)
**Abnahme:** In beiden Exchange-Umgebungen Postfächer über den zentralen Zugang ermitteln, eine Auswahl treffen und vollständig sowie inkrementell sichern. Die gesicherten Nachrichten müssen offline durchsuchbar sein. Prüfen, dass nicht ausgewählte Postfächer unberührt bleiben und fehlende oder entzogene Berechtigungen sichtbar gemeldet werden.

---

## 21. Spätere Ausbaustufe nach Abnahme der Grundversion

1. **Verschlüsselung archivierter Postfächer.** Archivdaten einschließlich Nachrichten, Metadaten, Suchindex und Zweitsicherungen verschlüsseln. Schlüsselverwaltung, Schlüsselwiederherstellung und die verifizierte Umstellung bestehender Archive vorsehen. Die Wiederherstellung auf einem Ersatzrechner mit gesichertem Schlüssel erproben.
2. **Eigenständige und unveränderbare Archive.** Archivbestände mit Nachrichten, Metadaten und Integritätsnachweisen separat verwalten, exportieren und auf einem anderen Rechner wieder einbinden. Zusätzlich abgeschlossene Bestände für eine definierte Dauer gegen Änderung und Löschung schützen. Die Unveränderbarkeit auch auf der Speicherebene gegen das vereinbarte Bedrohungsmodell prüfen. Ein Integritätsnachweis macht nachträgliche Veränderungen erkennbar, während eine Speichersperre sie verhindern soll. [MailStore-Referenz für Archivspeicher](https://help.mailstore.com/en/server/index.php?title=Storage_Locations)
3. **Datenschutzgerechte Archivierung.** Rollen und Zugriffsbeschränkungen je Archiv, nachvollziehbare Zugriffs- und Exportprotokolle, zweckgebundene Aufbewahrungsfristen, Löschsperren und Workflows für Auskunft und Löschung umsetzen. Vor der Umsetzung unveränderbarer Archive die Regeln für Fristablauf, Löschsperren und Zweitsicherungen gemeinsam festlegen. Die Abnahme umfasst dokumentierte Verarbeitungszwecke, Rechtsgrundlagen und Fristen für den konkreten Einsatz sowie Tests für Berechtigungen, Export und Löschung. [DSGVO auf EUR-Lex](https://eur-lex.europa.eu/eli/reg/2016/679/oj/eng/) · [MailStore-Referenz für Aufbewahrung und Löschsperren](https://help.mailstore.com/en/server/Compliance_General)
4. **Benutzer und Postfachberechtigungen.** Administratoren können im Admin-Client weitere lokale Benutzer anlegen, deaktivieren und ihnen je Postfach getrennt das Lesen und das Wiederherstellen erlauben. Das Leserecht umfasst Ordner, Suche, Nachrichtenansicht sowie EML- und Anhangdownloads. Bei einem Restore müssen sowohl der ausgewählte Archivbestand als auch das Zielpostfach für die jeweilige Aktion freigegeben sein. Rechte serverseitig für jeden Zugriff prüfen, auch bei direkten Links und Hintergrundaufträgen. Standardmäßig erhalten neue Benutzer keinen Zugriff auf Postfächer. Änderungen an Berechtigungen protokollieren und mit Tests für verweigerte Zugriffe, Suchtreffer, Downloads und Wiederherstellung abnehmen.
5. **Admin-Client und Benutzer-Client für Windows und Linux.** Zwei getrennt ausgelieferte Desktop-Anwendungen öffnen die jeweils passende, vom MailArchive-Server ausgelieferte Oberfläche in einem nativen WebView-Fenster. Die Serverdistribution enthält den Admin-Client für Postfachverwaltung, Einstellungen, Aufträge und Benutzerfreigaben. Der separat installierbare Benutzer-Client verbindet sich mit einer konfigurierbaren Serveradresse und zeigt nach Anmeldung ausschließlich die freigegebenen Archive und erlaubten Aktionen an. Administrative Bereiche sind der Administratorrolle vorbehalten, auch bei direkten Anfragen aus dem Benutzer-Client.

   Wails ist der bevorzugte Kandidat für beide Anwendungen. Vor der Umsetzung Laufzeitvoraussetzungen, Servernavigation und Downloads auf Windows und Linux erproben. Anmeldung, Sitzungsverwaltung und sämtliche Berechtigungsprüfungen liegen beim Server. Den Netzwerkzugriff ausdrücklich aktivieren und durch HTTPS mit Zertifikatsprüfung absichern. Die WebView auf die konfigurierte Server-Origin begrenzen. Die geladene Oberfläche erhält keine privilegierten nativen Client-Bindings. Für die Abnahme im Admin-Client ein Archiv freigeben, im Benutzer-Client die Sichtbarkeit und erlaubten Aktionen prüfen und anschließend den Entzug des Zugriffs auch über direkte Anfragen verifizieren.

CREATE VIRTUAL TABLE archive_search USING fts5(sender, recipient, subject, body, tokenize='unicode61');
CREATE TABLE archive_search_pending (
 message_id INTEGER PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE
) STRICT;
INSERT INTO archive_search_pending SELECT id FROM messages;

CREATE TRIGGER queue_message_index AFTER INSERT ON messages BEGIN
 INSERT INTO archive_search_pending(message_id) VALUES (new.id);
END;

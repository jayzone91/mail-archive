package retention

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jayzone91/mail-archive/internal/database"
)

func TestCutoffClampsShortMonth(t *testing.T) {
	now := time.Date(2024, time.April, 30, 13, 42, 0, 0, time.FixedZone("test", 3600))
	want := time.Date(2024, time.February, 29, 13, 42, 0, 0, now.Location())
	if got := Cutoff(now, 2); !got.Equal(want) {
		t.Fatalf("Stichtag: %v, erwartet %v", got, want)
	}
}

func TestPreviewExcludesRecentlyRestoredAndObsoleteUIDs(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "retention.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO accounts(id,name,email,username,host,port,tls_mode,enabled,auto_backup)
 VALUES ('target','Target','target@example.test','test','localhost',993,'implicit',1,1);
 INSERT INTO folders(account_id,name,delimiter,attributes,uid_validity,uid_next) VALUES ('target','INBOX','/','[]',7,5);
 INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
 VALUES (1,7,1,printf('%064d',1),1,'2020-01-01T00:00:00Z','[]','null'),
 (1,7,2,printf('%064d',2),1,'2020-01-01T00:00:00Z','[]','null'),
 (1,6,3,printf('%064d',3),1,'2020-01-01T00:00:00Z','[]','null'),
 (1,7,4,printf('%064d',4),1,'2026-01-25T00:00:00Z','[]','null');
 INSERT INTO restore_history(target,folder,message_id,validity,first_uid,target_uid,restored_at)
 VALUES ('target','INBOX',2,7,2,2,'2026-01-25 00:00:00');`)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := Preview(ctx, db, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(groups) != 1 || groups[0].Total != 1 || len(groups[0].Sample) != 1 || groups[0].Sample[0].MessageID != 1 {
		t.Fatalf("Aufbewahrungsvorschau: %+v, %v", groups, err)
	}
}

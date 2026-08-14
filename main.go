// Command jira-home runs the whole app as a single binary: SQLite file,
// server-rendered templates, and static assets are all embedded — nothing
// to install or configure beyond where the database file should live.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"jira-home/internal/store"
	"jira-home/internal/web"
)

func main() {
	dbPath := os.Getenv("JIRA_HOME_DB")
	if dbPath == "" {
		dbPath = "data/jira-home.db"
	}
	addr := os.Getenv("JIRA_HOME_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if err := st.EnsureDefaultProject(); err != nil {
		log.Fatalf("ensure default project: %v", err)
	}

	server := web.NewServer(st)

	// Sprint auto-cycling (Settings > Workspace) needs no external cron —
	// checking hourly is plenty for day-granularity sprint boundaries, and
	// running once immediately at startup means a rollover due while the
	// app was stopped still happens promptly on the next launch.
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			if err := st.RunSprintAutoCycle(); err != nil {
				log.Printf("sprint auto-cycle: %v", err)
			}
			<-ticker.C
		}
	}()

	log.Printf("Jira Home listening on %s (db: %s)", addr, dbPath)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatal(err)
	}
}

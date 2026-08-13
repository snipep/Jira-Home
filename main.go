// Command jira-home runs the whole app as a single binary: SQLite file,
// server-rendered templates, and static assets are all embedded — nothing
// to install or configure beyond where the database file should live.
package main

import (
	"log"
	"net/http"
	"os"

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

	log.Printf("Jira Home listening on %s (db: %s)", addr, dbPath)
	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatal(err)
	}
}

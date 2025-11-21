package http

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"pr-reviewer-service/internal/repository"
	"pr-reviewer-service/internal/service"
)

func startPostgres(t *testing.T) (dsn string, terminate func()) {
	t.Helper()
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
			"POSTGRES_DB":       "pr_reviewer",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithStartupTimeout(60 * time.Second),
	}
	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start container: %v", err)
	}
	terminate = func() { _ = pg.Terminate(ctx) }

	host, err := pg.Host(ctx)
	if err != nil {
		terminate()
		t.Fatalf("get host: %v", err)
	}
	port, err := pg.MappedPort(ctx, "5432")
	if err != nil {
		terminate()
		t.Fatalf("map port: %v", err)
	}
	dsn = fmt.Sprintf("postgres://postgres:postgres@%s:%s/pr_reviewer?sslmode=disable", host, port.Port())
	return dsn, terminate
}

func applyMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	for i := 0; i < 10; i++ {
		if err := db.Ping(); err == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "migrations", "001_init.sql")
	schema, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}

func TestIntegration_CreateReassignMerge(t *testing.T) {
	dsn, terminate := startPostgres(t)
	defer terminate()

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	applyMigration(t, db)

	teamsRepo := repository.NewTeamsRepo(db)
	usersRepo := repository.NewUsersRepo(db)
	prsRepo := repository.NewPRsRepo(db)
	teamsSvc := service.NewTeamsService(teamsRepo)
	usersSvc := service.NewUsersService(usersRepo)
	prsSvc := service.NewPRService(prsRepo, usersRepo, db)
	h := NewHandler(teamsSvc, usersSvc, prsSvc)
	router := NewRouter(h)
	ts := httptest.NewServer(router)
	defer ts.Close()

	do := func(method, path string, body io.Reader, want int) []byte {
		req, _ := http.NewRequest(method, ts.URL+path, body)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s error: %v", method, path, err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("%s %s unexpected status %d body=%s", method, path, res.StatusCode, string(b))
		}
		return b
	}

	// create team with active u2,u3 for replacement
do(http.MethodPost, "/team/add", bytes.NewBufferString(`{
        "team_name":"backend",
        "members":[
            {"user_id":"u1","username":"Alice","is_active":true},
            {"user_id":"u2","username":"Bob","is_active":true},
            {"user_id":"u3","username":"Carol","is_active":true},
            {"user_id":"u4","username":"Dave","is_active":true}
        ]}`), http.StatusCreated)

	// create PR
	body := do(http.MethodPost, "/pullRequest/create", bytes.NewBufferString(`{
        "pull_request_id":"pr-int-1",
        "pull_request_name":"feat-int",
        "author_id":"u1"
    }`), http.StatusCreated)
	var created struct {
		PR struct {
			Assigned []string `json:"assigned_reviewers"`
		} `json:"pr"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode create pr: %v", err)
	}
	if len(created.PR.Assigned) == 0 {
		t.Fatalf("expected reviewers assigned")
	}

	old := created.PR.Assigned[0]
	payload := fmt.Sprintf(`{"pull_request_id":"pr-int-1","old_user_id":"%s"}`, old)
	do(http.MethodPost, "/pullRequest/reassign", bytes.NewBufferString(payload), http.StatusOK)

	// merge is idempotent
	res1 := do(http.MethodPost, "/pullRequest/merge", bytes.NewBufferString(`{"pull_request_id":"pr-int-1"}`), http.StatusOK)
	res2 := do(http.MethodPost, "/pullRequest/merge", bytes.NewBufferString(`{"pull_request_id":"pr-int-1"}`), http.StatusOK)
	if !bytes.Equal(res1, res2) {
		t.Fatalf("merge not idempotent, responses differ")
	}
}

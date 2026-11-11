//go:build integration

package setup

import (
	"context"
	"os/exec"
	"testing"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestDatabaseConnectionCreatesMissingTargetDatabase(t *testing.T) {
	ctx := context.Background()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for the PostgreSQL integration test")
	}
	container, err := tcpostgres.Run(ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start temporary PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get temporary PostgreSQL host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("get temporary PostgreSQL port: %v", err)
	}

	if err := TestDatabaseConnection(&DatabaseConfig{
		Host: host, Port: port.Int(), User: "postgres", Password: "postgres",
		DBName: "sub2api_missing_target", SSLMode: "disable",
	}); err != nil {
		t.Fatalf("connect to maintenance database and create missing target: %v", err)
	}
}

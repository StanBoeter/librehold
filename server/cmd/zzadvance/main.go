package main

// Advances the world's clock by moving its started_at back, so a real battle resolves. This is a dev-world
// shortcut, not a game feature: WorldNow = wallMs - StartedAtMs - PausedTotalMs, so shifting StartedAt into the
// past moves world time forward exactly.
import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	ms := os.Args[1]
	db, err := sql.Open("pgx", "postgres://librehold:librehold@localhost:5433/librehold?sslmode=disable")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	res, err := db.Exec("update worlds set started_at_ms = started_at_ms - $1::bigint where id = 2", ms)
	if err != nil {
		panic(err)
	}
	n, _ := res.RowsAffected()
	fmt.Println("worlds updated:", n)
}

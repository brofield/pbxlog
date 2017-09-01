package main

import (
	"bufio"
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	conn, err := net.Dial("tcp", "192.168.1.250:5100")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	connbuf := bufio.NewReader(conn)

	f, err := os.OpenFile("calls.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	panicErr(err)
	defer f.Close()

	db, err := sql.Open("sqlite3", "calls.sqlite3")
	panicErr(err)
	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS calls (callid INTEGER, extn INTEGER, auth TEXT, ts TEXT, durn TEXT, " +
		"code TEXT, dialed TEXT, account TEXT, cost REAL, clid TEXT, clidname TEXT, gpno TEXT, ring TEXT);")
	panicErr(err)

	insert, err := db.Prepare("insert into CALLS(callid, extn, auth, ts, durn, code, dialed, account, cost, clid, clidname, gpno, ring) " +
		"values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	panicErr(err)
	defer insert.Close()

	tag := "===="
	for {
		str, err := connbuf.ReadString('\000')
		panicErr(err)
		f.Write([]byte(str))

		// skip the report header
		if str[0] == 0x0C {
			n := strings.LastIndex(str, tag)
			if n >= 0 {
				str = str[n+len(tag)+2:]
			}
		}

		n := len(str)
		if n > 0 {
			t := time.Now()

			callid, _ := strconv.Atoi(strings.TrimSpace(str[2:8]))
			extn, _ := strconv.Atoi(strings.TrimSpace(str[9:15]))
			auth := strings.TrimSpace(str[16:25])
			ts := strings.TrimSpace(fmt.Sprintf("%d-%s-%s", t.Year(), str[26:28], str[29:40]))
			durn := strings.TrimSpace(str[41:49])
			code := strings.TrimSpace(str[50:52])
			dialed := strings.TrimSpace(str[53:71])
			account := strings.TrimSpace(str[72:89])
			cost := strings.TrimSpace(str[90:100])
			clid := strings.TrimSpace(str[101:117])
			clidname := strings.TrimSpace(str[118:136])
			gpno := strings.TrimSpace(str[137:143])
			ring := strings.TrimSpace(str[143:151])

			_, err = insert.Exec(callid, extn, auth, ts, durn, code, dialed, account, cost, clid, clidname, gpno, ring)
			panicErr(err)

			fmt.Print(".")
		}
	}
}

func panicErr(err error, args ...string) {
	if err != nil {
		panic(fmt.Sprintf("Error: %q: %s\n", err, args))
	}
}

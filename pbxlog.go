package main

import (
	"bufio"
	"database/sql"
	"fmt"
	_ "github.com/kardianos/service"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/viper"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func loadConfig() {
	viper.SetConfigName("pbxlog")
	viper.SetConfigType("yaml")

	viper.AddConfigPath("$HOME") 
	viper.AddConfigPath("$HOME/.pbxlog")
	viper.AddConfigPath(".")

	viper.SetDefault("pabx", "")
	viper.SetDefault("dump-file", "")
	viper.SetDefault("calls-db", "")

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	fmt.Printf("Using pabx = %s\n", viper.GetString("pabx"))
	fmt.Printf("Using dump-file = %s\n", viper.GetString("dump-file"))
	fmt.Printf("Using calls-db = %s\n", viper.GetString("calls-db"))

	if viper.GetString("pabx") == "" || viper.GetString("calls-db") == "" {
		panic("pabx and calls-db configuration values required")
	}
}

func main() {
	loadConfig()

	conn, err := net.Dial("tcp", viper.GetString("pabx"))
	panicErr(err)
	connbuf := bufio.NewReader(conn)

	var dumpfile *os.File = nil
	if viper.GetString("dump-file") != "" {
		dumpfile, err = os.OpenFile(viper.GetString("dump-file"), 
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		panicErr(err)
		defer dumpfile.Close()
	}

	db, err := sql.Open("sqlite3", viper.GetString("calls-db"))
	panicErr(err)
	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS calls (" +
		"callid INTEGER, extn INTEGER, auth TEXT, ts TEXT, durn TEXT, " +
		"code TEXT, dialed TEXT, account TEXT, cost REAL, clid TEXT, " +
		"clidname TEXT, gpno TEXT, ring TEXT);")
	panicErr(err)

	insert, err := db.Prepare("insert into CALLS(callid, extn, auth, ts, " +
		"durn, code, dialed, account, cost, clid, clidname, gpno, ring) " +
		"values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	panicErr(err)
	defer insert.Close()

	tag := "===="
	for {
		str, err := connbuf.ReadString('\000')
		panicErr(err)
		if dumpfile != nil {
			dumpfile.Write([]byte(str))
		}

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

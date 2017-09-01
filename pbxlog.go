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

func connectToPABX() net.Conn {
	conn, err := net.Dial("tcp", viper.GetString("pabx"))
	panicErr(err)
	return conn
}

func openDumpFile() *os.File {
	if viper.GetString("dump-file") == "" {
		return nil
	}

	dumpfile, err := os.OpenFile(viper.GetString("dump-file"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	panicErr(err)

	return dumpfile
}

func openCallsDatabase() (*sql.DB, *sql.Stmt) {
	db, err := sql.Open("sqlite3", viper.GetString("calls-db"))
	panicErr(err)

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS calls (" +
		"callid INTEGER, extn INTEGER, auth TEXT, ts TEXT, durn TEXT, " +
		"code TEXT, dialed TEXT, account TEXT, cost REAL, clid TEXT, " +
		"clidname TEXT, gpno TEXT, ring TEXT);")
	panicErr(err)

	insert, err := db.Prepare("insert into CALLS(callid, extn, auth, ts, " +
		"durn, code, dialed, account, cost, clid, clidname, gpno, ring) " +
		"values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	panicErr(err)

	return db, insert
}

type CDR struct {
	callid    int
	extension int
	auth      string
	calltime  string
	duration  string
	code      string
	dialed    string
	account   string
	cost      string
	clid      string
	clidname  string
	gpno      string
	ringtime  string
}

func splitData(str string) *CDR {
	t := time.Now()

	var cdr CDR
	cdr.callid, _ = strconv.Atoi(strings.TrimSpace(str[2:8]))
	cdr.extension, _ = strconv.Atoi(strings.TrimSpace(str[9:15]))
	cdr.auth = strings.TrimSpace(str[16:25])
	cdr.calltime = strings.TrimSpace(fmt.Sprintf("%d-%s-%s", t.Year(), str[26:28], str[29:40]))
	cdr.duration = strings.TrimSpace(str[41:49])
	cdr.code = strings.TrimSpace(str[50:52])
	cdr.dialed = strings.TrimSpace(str[53:71])
	cdr.account = strings.TrimSpace(str[72:89])
	cdr.cost = strings.TrimSpace(str[90:100])
	cdr.clid = strings.TrimSpace(str[101:117])
	cdr.clidname = strings.TrimSpace(str[118:136])
	cdr.gpno = strings.TrimSpace(str[137:143])
	cdr.ringtime = strings.TrimSpace(str[143:151])

	return &cdr
}

func skipHeader(line string) string {
	const tag = "===="
	if line[0] == 0x0C {
		n := strings.LastIndex(line, tag)
		if n >= 0 {
			return line[n+len(tag)+2:]
		}
	}
	return line
}

func main() {
	loadConfig()

	dumpfile := openDumpFile()
	if dumpfile != nil {
		defer dumpfile.Close()
	}

	db, dbins := openCallsDatabase()
	defer db.Close()
	defer dbins.Close()

	pabxConn := connectToPABX()
	defer pabxConn.Close()
	pabx := bufio.NewReader(pabxConn)

	for {
		// every call record ends with a nul character
		str, err := pabx.ReadString('\000')
		panicErr(err)
		if dumpfile != nil {
			dumpfile.Write([]byte(str))
		}
		fmt.Print(".")

		// every now and then the PABX will preface the call
		// record with a human readable header. Skip it.
		str = skipHeader(str)

		// process this single call record
		cdr := splitData(str)
		_, err = dbins.Exec(cdr.callid, cdr.extension, cdr.auth,
			cdr.calltime, cdr.duration, cdr.code, cdr.dialed, cdr.account,
			cdr.cost, cdr.clid, cdr.clidname, cdr.gpno, cdr.ringtime)
		panicErr(err)
	}
}

func panicErr(err error, args ...string) {
	if err != nil {
		panic(fmt.Sprintf("Error: %q: %s\n", err, args))
	}
}

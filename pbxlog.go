package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"github.com/kardianos/osext"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/viper"
	"html/template"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type DbContext struct {
	Db     *sql.DB
	Insert *sql.Stmt
}

func (c *DbContext) Close() {
	if c.Insert != nil {
		c.Insert.Close()
	}
	if c.Db != nil {
		c.Db.Close()
	}
}

func loadConfig() {
	viper.SetConfigName("pbxlog")
	viper.SetConfigType("yaml")

	viper.AddConfigPath("$HOME")
	viper.AddConfigPath("$HOME/.pbxlog")
	viper.AddConfigPath(".")

	viper.SetDefault("pabx", "")
	viper.SetDefault("webui", "")
	viper.SetDefault("dump-file", "")
	viper.SetDefault("error-file", "")
	viper.SetDefault("calls-db", "")

	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	fmt.Printf("Using pabx = %s\n", viper.GetString("pabx"))
	fmt.Printf("Using webui = %s\n", viper.GetString("webui"))
	fmt.Printf("Using dump-file = %s\n", viper.GetString("dump-file"))
	fmt.Printf("Using error-file = %s\n", viper.GetString("error-file"))
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

func openDumpFile(configItem string) *os.File {
	filename := viper.GetString(configItem)
	if filename == "" {
		return nil
	}

	dumpfile, err := os.OpenFile(filename,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	panicErr(err)

	return dumpfile
}

type CDR struct {
	Callid    int
	Extension int
	Auth      string
	Calltime  string
	Duration  string
	Code      string
	Dialed    string
	Account   string
	Cost      string
	Clid      string
	Clidname  string
	Gpno      string
	Ringtime  string
}

func openCallsDatabase() *DbContext {
	ctx := new(DbContext)

	var err error
	ctx.Db, err = sql.Open("sqlite3", viper.GetString("calls-db"))
	panicErr(err)

	_, err = ctx.Db.Exec("CREATE TABLE IF NOT EXISTS calls (" +
		"callid INTEGER, extension INTEGER, auth TEXT, calltime TEXT, duration TEXT, " +
		"code TEXT, dialed TEXT, account TEXT, cost REAL, clid TEXT, " +
		"clidname TEXT, gpno TEXT, ringtime TEXT);")
	panicErr(err)

	ctx.Insert, err = ctx.Db.Prepare("insert into CALLS(callid, extension, auth, calltime, " +
		"duration, code, dialed, account, cost, clid, clidname, gpno, ringtime) " +
		"values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	panicErr(err)

	return ctx
}

func insertCDR(ctx *DbContext, cdr *CDR) {
	_, err := ctx.Insert.Exec(cdr.Callid, cdr.Extension, cdr.Auth,
		cdr.Calltime, cdr.Duration, cdr.Code, cdr.Dialed, cdr.Account,
		cdr.Cost, cdr.Clid, cdr.Clidname, cdr.Gpno, cdr.Ringtime)
	panicErr(err)
}

func splitData(str string) *CDR {
	t := time.Now()
	if len(str) != 154 {
		return nil
	}

	var cdr CDR
	cdr.Callid, _ = strconv.Atoi(strings.TrimSpace(str[2:8]))
	cdr.Extension, _ = strconv.Atoi(strings.TrimSpace(str[9:15]))
	cdr.Auth = strings.TrimSpace(str[16:25])
	cdr.Calltime = strings.TrimSpace(fmt.Sprintf("%d-%s-%s", t.Year(), str[26:28], str[29:40]))
	cdr.Duration = strings.TrimSpace(str[41:49])
	cdr.Code = strings.TrimSpace(str[50:52])
	cdr.Dialed = strings.TrimSpace(str[53:71])
	cdr.Account = strings.TrimSpace(str[72:89])
	cdr.Cost = strings.TrimSpace(str[90:100])
	cdr.Clid = strings.TrimSpace(str[101:117])
	cdr.Clidname = strings.TrimSpace(str[118:136])
	cdr.Gpno = strings.TrimSpace(str[137:143])
	cdr.Ringtime = strings.TrimSpace(str[143:151])

	return &cdr
}

func skipHeader(line string) string {
	const tag = "===="
	if line[0] == 0x0C {
		n := strings.LastIndex(line, tag)
		if n >= 0 {
			fmt.Print(" HDR ")
			return line[n+len(tag)+2:]
		}
	}
	return line
}

func main() {
	loadConfig()

	pabxConn := connectToPABX()
	defer pabxConn.Close()
	pabx := bufio.NewReader(pabxConn)

	ctx := openCallsDatabase()
	defer ctx.Close()

	startWebServer(ctx)

	dumpfile := openDumpFile("dump-file")
	if dumpfile != nil {
		defer dumpfile.Close()
	}

	for {
		// every call record ends with a nul character
		str, err := pabx.ReadString('\000')
		panicErr(err)
		if dumpfile != nil {
			dumpfile.Write([]byte(str))
			dumpfile.Sync()
		}
		fmt.Print(".")

		// every now and then the PABX will preface the call
		// record with a human readable header. Skip it.
		str = skipHeader(str)

		// process this single call record
		cdr := splitData(str)
		if cdr == nil {
			dumpError(str)
		} else {
			insertCDR(ctx, cdr)
		}
	}
}

func dumpError(str string) {
	fmt.Print(" INVALID ")
	errorfile := openDumpFile("error-file")
	if errorfile != nil {
		defer errorfile.Close()
		msg := fmt.Sprintf("\nError: len = %d\n--\n%s\n--\n", len(str), str)
		errorfile.Write([]byte(msg))
		errorfile.Sync()
	}
}

func panicErr(err error, args ...string) {
	if err != nil {
		panic(fmt.Sprintf("Error: %q: %s\n", err, args))
	}
}

/* ----- WEBUI ----- */

func startWebServer(ctx *DbContext) {
	webui := viper.GetString("webui")
	if webui == "" {
		return
	}

	http.HandleFunc("/",
		func(w http.ResponseWriter, r *http.Request) {
			handler(ctx, w, r)
		})

	listener, err := net.Listen("tcp", webui)
	panicErr(err)

	go http.Serve(listener, nil)
}

type Row struct {
	CDR
	Group int
}

func handler(ctx *DbContext, w http.ResponseWriter, r *http.Request) {
	folder, err := osext.ExecutableFolder()
	if err != nil {
		fmt.Fprintf(w, "osext = %v", err)
		return
	}
	templateFile := path.Join(folder, "pbxlog.html")

	t := template.New("pbxlog.html")
	t, err = t.ParseFiles(templateFile)
	if err != nil {
		fmt.Fprintf(w, "ParseFiles = %v", err)
		return
	}

	limit, err := strconv.Atoi(r.FormValue("limit"))
	if err != nil || limit < 1 {
		limit = 200
	}

	offset, err := strconv.Atoi(r.FormValue("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}

	rows, err := ctx.Db.Query(
		"SELECT callid, extension, auth, calltime, "+
			"duration, code, dialed, account, cost, clid, clidname, gpno, ringtime "+
			"FROM calls ORDER BY callid DESC, calltime DESC LIMIT ? OFFSET ?;",
		limit, offset)
	if err != nil {
		fmt.Fprintf(w, "Query = %v", err)
		return
	}
	defer rows.Close()

	var lst []Row
	var cdr Row
	var lastCallid int
	for rows.Next() {
		err = rows.Scan(&cdr.Callid, &cdr.Extension, &cdr.Auth, &cdr.Calltime,
			&cdr.Duration, &cdr.Code, &cdr.Dialed, &cdr.Account, &cdr.Cost,
			&cdr.Clid, &cdr.Clidname, &cdr.Gpno, &cdr.Ringtime)
		if lastCallid != cdr.Callid {
			lastCallid = cdr.Callid
			cdr.Group = (cdr.Group + 1) % 2
		}
		if err != nil {
			fmt.Fprintf(w, "rows.Scan() = %v", err)
		}
		lst = append(lst, cdr)
	}
	err = rows.Err()
	if err != nil {
		fmt.Fprintf(w, "rows.Err() = %v", err)
	}

	err = t.Execute(w, lst)
	if err != nil {
		fmt.Fprintf(w, "%v", err)
		return
	}
}

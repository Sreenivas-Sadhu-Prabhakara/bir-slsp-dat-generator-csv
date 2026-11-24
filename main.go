// Command bir-slsp-dat-generator-csv turns a sales or purchases CSV into the
// data file a value-added-tax registered taxpayer in the Philippines submits
// as the Summary List of Sales and Purchases.
//
// It serves a small web interface on the loopback address and keeps its
// settings and its recent runs in one JSON file on the same machine. It makes
// no network calls of its own: the customer and supplier details it is given
// never leave the computer it runs on.
//
// It can also be run once from the command line, which is what a bookkeeper
// scripting a month-end close wants:
//
//	bir-slsp-dat-generator-csv -csv sales-september.csv -list sales \
//	    -period 09/2026 -tin 123-456-789-000 -name "ACME TRADING CORPORATION" \
//	    -out ./
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sreenivas-sadhu-prabhakara/bir-slsp-dat-generator-csv/internal/slsp"
)

//go:embed web
var webFiles embed.FS

const (
	defaultAddr = "127.0.0.1:8080"
	defaultData = "./bir-slsp-dat-generator-csv-data.json"

	// maxUploadBytes caps one request. A summary list for a small business is
	// a few hundred kilobytes.
	maxUploadBytes = 64 << 20
	// maxSavedRuns is how many past runs the data file keeps.
	maxSavedRuns = 25
	// dataFileVersion is the shape of the data file, for future upgrades.
	dataFileVersion = 1

	// contentSecurityPolicy is sent with the interface as well as being set in
	// the page itself: everything the page loads comes from this binary.
	contentSecurityPolicy = "default-src 'self'; base-uri 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; font-src 'self'; form-action 'self'; connect-src 'self'"
)

func main() {
	addr := flag.String("addr", defaultAddr, "address to serve the interface on; keep it on 127.0.0.1 unless you know you want it reachable")
	data := flag.String("data", defaultData, "file this tool keeps its settings and recent runs in")

	csvPath := flag.String("csv", "", "run once instead of serving: the sales or purchases CSV to read")
	out := flag.String("out", ".", "with -csv: the directory to write the data file into")
	listName := flag.String("list", "sales", "with -csv: which list to produce, sales or purchases")
	period := flag.String("period", "", "with -csv: the month the list covers, for example 09/2026")
	tinFlag := flag.String("tin", "", "with -csv: the TIN of the taxpayer filing the list")
	nameFlag := flag.String("name", "", "with -csv: the registered name of the taxpayer filing the list")
	personFlag := flag.String("person", "", "with -csv: an individual filer's name, written SURNAME, First Middle")
	tradeFlag := flag.String("trade-name", "", "with -csv: the business name the filer trades under, if different")
	addrLines := flag.String("address", "", "with -csv: the filer's address, as two parts separated by | — street and barangay, then city and postal code")
	rdoFlag := flag.String("rdo", "", "with -csv: the filer's revenue district office code, in exactly three digits, for example 044")
	yearEnd := flag.Int("year-end", 12, "with -csv: the month the filer's accounting year ends in, 1 to 12; 12 is a calendar year, 0 leaves it blank")
	nonCreditable := flag.String("non-creditable", "0", "with -csv on a purchases list: how much of the month's input tax is not creditable")
	dateOrder := flag.String("date-order", "month-first", "how to read a numeric date in the CSV: month-first or day-first")
	vatRate := flag.Int("vat-rate", slsp.VATRatePercent, "the rate of value-added tax used to check the tax columns, as a whole percentage")
	allowNeg := flag.Bool("allow-negative", false, "let a negative amount through; a summary list normally has no column for one")
	noMerge := flag.Bool("no-consolidate", false, "keep one record per CSV row instead of adding together the rows naming the same taxpayer")
	showLayout := flag.Bool("layout", false, "print the record layout of the data file and exit")
	writeSample := flag.String("sample", "", "write a CSV template for the named list (sales or purchases) to standard output and exit")
	flag.Parse()

	if *showLayout {
		printLayout(os.Stdout)
		return
	}
	if *writeSample != "" {
		k, ok := slsp.ParseListKind(*writeSample)
		if !ok {
			log.Fatalf("bir-slsp-dat-generator-csv: -sample takes sales or purchases, not %q", *writeSample)
		}
		fmt.Print(slsp.SampleCSV(k))
		return
	}
	if *csvPath != "" {
		code := runOnce(onceArgs{
			csvPath: *csvPath, out: *out, list: *listName, period: *period,
			tin: *tinFlag, name: *nameFlag, person: *personFlag, trade: *tradeFlag,
			address: *addrLines, rdo: *rdoFlag, yearEnd: *yearEnd, nonCreditable: *nonCreditable,
			dateOrder: *dateOrder, vatRate: *vatRate, allowNeg: *allowNeg, noMerge: *noMerge,
		})
		os.Exit(code)
	}

	st, err := openStore(*data)
	if err != nil {
		log.Fatalf("bir-slsp-dat-generator-csv: %v", err)
	}
	ui, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("bir-slsp-dat-generator-csv: %v", err)
	}

	srv := &server{store: st, ui: http.FileServerFS(ui)}
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 20 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("Summary List of Sales and Purchases generator ready at http://%s/ (data file: %s)", *addr, st.path)
	log.Printf("press Ctrl-C to stop")
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("bir-slsp-dat-generator-csv: %v", err)
	}
}

// ------------------------------------------------------------ run once

type onceArgs struct {
	csvPath, out, list, period string
	tin, name, person, trade   string
	address, rdo               string
	yearEnd                    int
	nonCreditable              string
	dateOrder                  string
	vatRate                    int
	allowNeg, noMerge          bool
}

// runOnce does one generation from the command line and returns the exit code:
// 0 if the data file was written, 1 if it was refused, 2 if the arguments or
// the CSV could not be used at all.
func runOnce(a onceArgs) int {
	opts, err := optionsFromFlags(a)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bir-slsp-dat-generator-csv: %v\n", err)
		return 2
	}
	f, err := os.Open(expandHome(a.csvPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bir-slsp-dat-generator-csv: could not open the CSV: %v\n", err)
		return 2
	}
	defer f.Close()

	res, err := slsp.Generate(f, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bir-slsp-dat-generator-csv: %v\n", err)
		return 2
	}
	for _, n := range res.Notes {
		fmt.Fprintf(os.Stderr, "note: %s\n", n)
	}
	for _, m := range res.Merges {
		fmt.Fprintf(os.Stderr, "merged: %s\n", m)
	}
	for _, fd := range res.Findings {
		fmt.Fprintf(os.Stderr, "%s\n", fd)
	}
	if !res.OK {
		fmt.Fprintf(os.Stderr, "\nThe data file was NOT written: %d error(s) must be fixed first.\n", res.FatalCount())
		return 1
	}
	dir := expandHome(a.out)
	dest := filepath.Join(dir, res.FileName)
	if err := writeFileAtomic(dest, res.DatBytes()); err != nil {
		fmt.Fprintf(os.Stderr, "bir-slsp-dat-generator-csv: could not write %s: %v\n", dest, err)
		return 2
	}
	// The submission copy goes in a folder of its own, the way the Bureau's
	// own validation module writes it, so the two are never confused.
	subDest := filepath.Join(dir, slsp.SubmissionFolder, res.FileName)
	if err := writeFileAtomic(subDest, res.SubmissionBytes()); err != nil {
		fmt.Fprintf(os.Stderr, "bir-slsp-dat-generator-csv: could not write %s: %v\n", subDest, err)
		return 2
	}
	fmt.Printf("%s\n", dest)
	fmt.Printf("%s\n", subDest)
	fmt.Fprintf(os.Stderr, "%d record(s), %d warning(s). Submission key %s.\n",
		res.Totals.Records, res.WarningCount(), res.ValidationKey)
	fmt.Fprintf(os.Stderr, "Run the file through the Bureau's own validation module before you submit it.\n")
	return 0
}

func optionsFromFlags(a onceArgs) (slsp.Options, error) {
	opts := slsp.DefaultOptions()
	k, ok := slsp.ParseListKind(a.list)
	if !ok {
		return opts, fmt.Errorf("-list takes sales or purchases, not %q", a.list)
	}
	opts.List = k

	p, err := slsp.ParsePeriod(a.period)
	if err != nil {
		return opts, fmt.Errorf("-period: %w", err)
	}
	opts.Period = p

	tin, err := slsp.ParseTIN(a.tin)
	if err != nil {
		return opts, fmt.Errorf("-tin: %w", err)
	}
	order, err := slsp.ParseDateOrder(a.dateOrder)
	if err != nil {
		return opts, fmt.Errorf("-date-order: %w", err)
	}
	opts.DateOrder = order
	opts.VATRate = a.vatRate
	opts.AllowNegativeAmounts = a.allowNeg
	opts.NoConsolidate = a.noMerge

	nc, err := slsp.ParseAmount(a.nonCreditable)
	if err != nil {
		return opts, fmt.Errorf("-non-creditable: %w", err)
	}
	opts.NonCreditableInputVAT = nc

	filer := slsp.Filer{
		TIN:                tin,
		TradeName:          slsp.CleanName(a.trade),
		RDOCode:            strings.TrimSpace(a.rdo),
		FiscalYearEndMonth: a.yearEnd,
	}
	switch {
	case strings.TrimSpace(a.person) != "":
		person, err := slsp.SplitPersonName(a.person)
		if err != nil {
			return opts, fmt.Errorf("-person: %w", err)
		}
		filer.Person = person
	case strings.TrimSpace(a.name) != "":
		filer.RegisteredName = slsp.CleanName(a.name)
	case filer.TradeName == "":
		return opts, errors.New("give the filing taxpayer's name with -name, or an individual's with -person")
	}
	filer.Address = splitAddress(a.address)
	opts.Filer = filer
	return opts, nil
}

// splitAddress spreads the address given on the command line over the two
// lines the record carries.
func splitAddress(s string) [slsp.AddressLines]string {
	var out [slsp.AddressLines]string
	if strings.TrimSpace(s) == "" {
		return out
	}
	parts := strings.Split(s, "|")
	for i := 0; i < len(out) && i < len(parts); i++ {
		out[i] = slsp.CleanName(parts[i])
	}
	// More parts than lines: fold the remainder into the last line rather
	// than losing it.
	if len(parts) > len(out) {
		tail := make([]string, 0, len(parts)-len(out)+1)
		tail = append(tail, out[len(out)-1])
		for _, p := range parts[len(out):] {
			tail = append(tail, slsp.CleanName(p))
		}
		out[len(out)-1] = strings.TrimSpace(strings.Join(tail, " "))
	}
	return out
}

// printLayout writes the record layout out in full, so that a user can compare
// it against a file produced by the Bureau's own module without reading the
// source.
func printLayout(w io.Writer) {
	for _, k := range []slsp.ListKind{slsp.Sales, slsp.Purchases} {
		d := slsp.Describe(k)
		fmt.Fprintf(w, "%s\n", strings.ToUpper(d.List))
		fmt.Fprintf(w, "  file name      %s\n", d.FileNamePattern)
		fmt.Fprintf(w, "  fields         separated by %q\n", d.FieldSeparator)
		fmt.Fprintf(w, "  text fields    enclosed in %s, always in the header, only when not empty in a detail record\n", d.TextDelimiter)
		fmt.Fprintf(w, "  amounts        2 decimal places; nil is 0.00 in the header and 0 in a detail record\n")
		fmt.Fprintf(w, "  records        ended by %s\n", d.RecordSeparator)
		fmt.Fprintf(w, "  encoding       %s\n\n", d.Encoding)
		for _, rec := range d.Records {
			fmt.Fprintf(w, "  %s record, first field %q, %d fields\n", rec.What, rec.Token, len(rec.Fields))
			for _, f := range rec.Fields {
				width := "-"
				if f.Max > 0 {
					width = fmt.Sprintf("%d", f.Max)
				}
				req := " "
				if f.Required {
					req = "*"
				}
				fmt.Fprintf(w, "    %3d %s %-38s %-7s %-4s %s\n", f.Position, req, f.Name, f.Kind, width, f.Confidence)
			}
			fmt.Fprintln(w)
		}
		if len(d.Uncertain) > 0 {
			fmt.Fprintf(w, "  Check these against a file produced by the Bureau's own module:\n")
			for _, u := range d.Uncertain {
				fmt.Fprintf(w, "    - %s\n", u)
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintf(w, "* required   widths are in characters, not counting the enclosing %s\n\n",
		slsp.Describe(slsp.Sales).TextDelimiter)

	// The per-field confidence above says how well each field could be
	// sourced. What it cannot say is how far the whole layout can be trusted,
	// so that goes here: somebody checking this tool against a real filing
	// should see the open questions on the same screen as the answers.
	fmt.Fprint(w, `WHERE THIS LAYOUT COMES FROM

  It was read off the Bureau of Internal Revenue's own RELIEF data entry
  module: the routine that writes the file, cross-checked field by field
  against the routine in the matching validation module that accepts or
  rejects it. Where the two agree, a field is marked sourced.

WHAT COULD NOT BE ESTABLISHED

  - The prescribing annexes to RMC 24-2002 could not be obtained in published
    form. This layout comes from the reference implementation instead.
  - Whether the module read here is the current one could not be confirmed.
    Check the version offered on the Bureau's own site before you rely on
    this for a filing.
  - No real, published .DAT file could be found to compare against byte for
    byte.
  - Whether electronic submission still requires the validation key line
    could not be confirmed, so both copies of the file are produced.
  - Record endings are a carriage return and a line feed, which follows from
    the file-writing call the module uses rather than from a published
    specification.
  - RR 16-2005 lists sales and purchases subject to final VAT withheld as
    required content, but this record layout has no field for them. The
    layout is followed, because that is what the validation module checks.

  Run every file through the Bureau's own validation module before you
  submit it.
`)
}

// ---------------------------------------------------------------- data file

// settings are the choices the user keeps between runs. The filer's own
// details change rarely, so keeping them saves retyping them every month.
type settings struct {
	TIN            string                    `json:"tin"`
	RegisteredName string                    `json:"registeredName"`
	PersonName     string                    `json:"personName"`
	TradeName      string                    `json:"tradeName"`
	Address        [slsp.AddressLines]string `json:"address"`
	RDOCode        string                    `json:"rdoCode"`
	YearEndMonth   int                       `json:"yearEndMonth"`
	Period         string                    `json:"period"`
	List           string                    `json:"list"`
	NonCreditable  string                    `json:"nonCreditable"`
	DateOrder      string                    `json:"dateOrder"`
	VATRate        int                       `json:"vatRate"`
	AllowNegative  bool                      `json:"allowNegative"`
	NoConsolidate  bool                      `json:"noConsolidate"`
}

func defaultSettings() settings {
	return settings{
		List:          "sales",
		DateOrder:     "month-first",
		VATRate:       slsp.VATRatePercent,
		YearEndMonth:  12,
		NonCreditable: "0",
	}
}

// run is one saved generation.
type run struct {
	ID        string       `json:"id"`
	CreatedAt time.Time    `json:"createdAt"`
	CSVName   string       `json:"csvName,omitempty"`
	Result    *slsp.Result `json:"result"`
}

// summary is the lightweight form of a run, for the list of saved runs.
type summary struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	CSVName   string    `json:"csvName,omitempty"`
	List      string    `json:"list"`
	Period    string    `json:"period"`
	FileName  string    `json:"fileName"`
	Records   int       `json:"records"`
	Errors    int       `json:"errors"`
	Warnings  int       `json:"warnings"`
	OK        bool      `json:"ok"`
}

type fileData struct {
	Version  int      `json:"version"`
	Settings settings `json:"settings"`
	Runs     []run    `json:"runs"`
}

type store struct {
	mu   sync.Mutex
	path string
	data fileData
}

func openStore(path string) (*store, error) {
	st := &store{path: path, data: fileData{Version: dataFileVersion, Settings: defaultSettings()}}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return st, nil
	case err != nil:
		return nil, fmt.Errorf("could not open the data file %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return st, nil
	}
	var loaded fileData
	if err := json.Unmarshal(b, &loaded); err != nil {
		return nil, fmt.Errorf("the data file %s is not readable (%v) — move it aside or point -data somewhere else", path, err)
	}
	if loaded.Settings.VATRate <= 0 {
		loaded.Settings.VATRate = slsp.VATRatePercent
	}
	if loaded.Settings.List == "" {
		loaded.Settings.List = "sales"
	}
	if loaded.Settings.DateOrder == "" {
		loaded.Settings.DateOrder = "month-first"
	}
	loaded.Version = dataFileVersion
	st.data = loaded
	return st, nil
}

// save writes the data file in one piece: a temporary file in the same
// directory, flushed to disk, then renamed over the old one, so an interrupted
// write cannot leave a half-written file behind.
func (s *store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, append(b, '\n'))
}

func writeFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func(e error) error {
		tmp.Close()
		os.Remove(name)
		return e
	}
	if _, err := tmp.Write(body); err != nil {
		return cleanup(err)
	}
	if err := tmp.Sync(); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func (s *store) getSettings() settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Settings
}

func (s *store) putSettings(next settings) (settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if next.VATRate <= 0 {
		next.VATRate = slsp.VATRatePercent
	}
	s.data.Settings = next
	return s.data.Settings, s.save()
}

func (s *store) addRun(r run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Runs = append([]run{r}, s.data.Runs...)
	if len(s.data.Runs) > maxSavedRuns {
		s.data.Runs = s.data.Runs[:maxSavedRuns]
	}
	return s.save()
}

func (s *store) run(id string) (run, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.data.Runs {
		if r.ID == id {
			return r, true
		}
	}
	return run{}, false
}

func (s *store) deleteRun(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.data.Runs {
		if r.ID == id {
			s.data.Runs = append(s.data.Runs[:i:i], s.data.Runs[i+1:]...)
			return true, s.save()
		}
	}
	return false, nil
}

func (s *store) summaries() []summary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]summary, 0, len(s.data.Runs))
	for _, r := range s.data.Runs {
		sm := summary{ID: r.ID, CreatedAt: r.CreatedAt, CSVName: r.CSVName}
		if r.Result != nil {
			sm.List = r.Result.List
			sm.Period = r.Result.Period
			sm.FileName = r.Result.FileName
			sm.Records = r.Result.Totals.Records
			sm.Errors = r.Result.FatalCount()
			sm.Warnings = r.Result.WarningCount()
			sm.OK = r.Result.OK
		}
		out = append(out, sm)
	}
	return out
}

func newRunID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102-150405.000")
	}
	return time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

// ------------------------------------------------------------------- server

type server struct {
	store *store
	ui    http.Handler
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/settings", s.handleSettings)
	mux.HandleFunc("POST /api/generate", s.handleGenerate)
	mux.HandleFunc("GET /api/run", s.handleRun)
	mux.HandleFunc("DELETE /api/run", s.handleDeleteRun)
	mux.HandleFunc("GET /api/dat", s.handleDat)
	mux.HandleFunc("GET /api/sample", s.handleSample)
	mux.Handle("/", s.ui)
	return headers(sameOriginOnly(mux))
}

// headers sends the same policy as the page's own meta tag, so the interface
// can only ever load what this binary serves.
func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// sameOriginOnly refuses a request that another web page tried to make to this
// tool. The interface is served from this binary, so its own requests always
// carry a matching origin or none at all.
func sameOriginOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				httpError(w, http.StatusForbidden, "this request did not come from the tool's own page")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					httpError(w, http.StatusForbidden, "this request did not come from the tool's own page")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("could not write the reply: %v", err)
	}
}

func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": s.store.getSettings(),
		"runs":     s.store.summaries(),
		"layout": map[string]any{
			"sales":     slsp.Describe(slsp.Sales),
			"purchases": slsp.Describe(slsp.Purchases),
		},
		"defaults": map[string]any{
			"vatRate":               slsp.VATRatePercent,
			"fileNamePattern":       slsp.FileNameTemplate,
			"maxNameChars":          slsp.MaxRegisteredNameChars,
			"maxPartyAddressChars":  slsp.MaxPartyAddressChars,
			"maxFilerAddress1Chars": slsp.MaxFilerAddress1Chars,
			"maxFilerAddress2Chars": slsp.MaxFilerAddress2Chars,
			"addressLines":          slsp.AddressLines,
			"tinDigits":             slsp.TINBaseDigits,
			"rdoCodeChars":          slsp.RDOCodeChars,
			"encoding":              slsp.EncodingName,
			"allowedCharacters":     slsp.AllowedCharacters(),
			"forbiddenCharacters":   slsp.ForbiddenCharacters(),
			"submissionFolder":      slsp.SubmissionFolder,
		},
	})
}

func (s *server) handleSettings(w http.ResponseWriter, r *http.Request) {
	var in settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		httpError(w, http.StatusBadRequest, "could not read the settings: "+err.Error())
		return
	}
	saved, err := s.store.putSettings(in)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not save the settings: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": saved})
}

// handleGenerate accepts the CSV either as an upload or as a path on this
// machine, generates the data file, saves the run and returns it.
func (s *server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	var (
		csvReader io.Reader
		csvName   string
		set       = s.store.getSettings()
		closers   []io.Closer
	)
	defer func() {
		for _, c := range closers {
			c.Close()
		}
	}()

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			httpError(w, http.StatusBadRequest, "could not read the uploaded file: "+err.Error())
			return
		}
		defer r.MultipartForm.RemoveAll()
		f, hdr, err := r.FormFile("csv")
		if err != nil {
			httpError(w, http.StatusBadRequest, "choose the CSV of your sales or purchases")
			return
		}
		closers = append(closers, f)
		csvReader, csvName = f, filepath.Base(hdr.Filename)
		applyFormSettings(&set, r.FormValue)
	} else {
		var in struct {
			settings
			CSVPath string `json:"csvPath"`
			CSVText string `json:"csvText"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			httpError(w, http.StatusBadRequest, "could not read the request: "+err.Error())
			return
		}
		set = in.settings
		switch {
		case strings.TrimSpace(in.CSVText) != "":
			csvReader, csvName = strings.NewReader(in.CSVText), "pasted rows"
		case strings.TrimSpace(in.CSVPath) != "":
			f, err := os.Open(expandHome(strings.TrimSpace(in.CSVPath)))
			if err != nil {
				httpError(w, http.StatusBadRequest, "could not open the CSV: "+err.Error())
				return
			}
			closers = append(closers, f)
			csvReader, csvName = f, filepath.Base(in.CSVPath)
		default:
			httpError(w, http.StatusBadRequest, "upload the CSV, give its path on this computer, or paste the rows")
			return
		}
	}

	opts, err := optionsFromSettings(set)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := slsp.Generate(csvReader, opts)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.store.putSettings(set); err != nil {
		log.Printf("could not save the settings: %v", err)
	}
	rec := run{ID: newRunID(), CreatedAt: time.Now().UTC(), CSVName: csvName, Result: res}
	if err := s.store.addRun(rec); err != nil {
		httpError(w, http.StatusInternalServerError, "the file was generated but the run could not be saved: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": rec})
}

func applyFormSettings(set *settings, get func(string) string) {
	str := func(key string, dst *string) {
		if v := get(key); v != "" {
			*dst = v
		}
	}
	str("tin", &set.TIN)
	str("registeredName", &set.RegisteredName)
	str("personName", &set.PersonName)
	str("tradeName", &set.TradeName)
	str("rdoCode", &set.RDOCode)
	str("period", &set.Period)
	str("list", &set.List)
	str("nonCreditable", &set.NonCreditable)
	str("dateOrder", &set.DateOrder)
	for i := range set.Address {
		str(fmt.Sprintf("address%d", i+1), &set.Address[i])
	}
	if v := get("yearEndMonth"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			set.YearEndMonth = n
		}
	}
	set.AllowNegative = isOn(get("allowNegative"))
	set.NoConsolidate = isOn(get("noConsolidate"))
}

func isOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

func optionsFromSettings(set settings) (slsp.Options, error) {
	return optionsFromFlags(onceArgs{
		list:          firstNonEmpty(set.List, "sales"),
		period:        set.Period,
		tin:           set.TIN,
		name:          set.RegisteredName,
		person:        set.PersonName,
		trade:         set.TradeName,
		address:       strings.Join(set.Address[:], "|"),
		rdo:           set.RDOCode,
		yearEnd:       set.YearEndMonth,
		nonCreditable: firstNonEmpty(set.NonCreditable, "0"),
		dateOrder:     firstNonEmpty(set.DateOrder, "month-first"),
		vatRate:       set.VATRate,
		allowNeg:      set.AllowNegative,
		noMerge:       set.NoConsolidate,
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (s *server) handleRun(w http.ResponseWriter, r *http.Request) {
	rec, ok := s.store.run(r.URL.Query().Get("id"))
	if !ok {
		httpError(w, http.StatusNotFound, "no saved run with that reference")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": rec})
}

func (s *server) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	removed, err := s.store.deleteRun(r.URL.Query().Get("id"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not save the data file: "+err.Error())
		return
	}
	if !removed {
		httpError(w, http.StatusNotFound, "no saved run with that reference")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDat serves a generated data file for download, in the single-byte
// encoding the format uses rather than as the browser's own text. A run that
// was refused has no file to serve.
//
// There are two files: the plain one, which is what the Bureau's validation
// module expects to be given, and the submission copy with the key line
// appended, which is what goes in through electronic submission.
func (s *server) handleDat(w http.ResponseWriter, r *http.Request) {
	rec, ok := s.store.run(r.URL.Query().Get("id"))
	if !ok || rec.Result == nil {
		httpError(w, http.StatusNotFound, "no saved run with that reference")
		return
	}
	if !rec.Result.OK || rec.Result.Dat == "" {
		httpError(w, http.StatusConflict, "that run was refused, so there is no data file to download")
		return
	}
	body, name := rec.Result.DatBytes(), rec.Result.FileName
	if r.URL.Query().Get("copy") == "submission" {
		body, name = rec.Result.SubmissionBytes(), rec.Result.FileName
	}
	w.Header().Set("Content-Type", "text/plain; charset=windows-1252")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		log.Printf("could not write the data file: %v", err)
	}
}

func (s *server) handleSample(w http.ResponseWriter, r *http.Request) {
	k, ok := slsp.ParseListKind(r.URL.Query().Get("list"))
	if !ok {
		k = slsp.Sales
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+slsp.SampleCSVName(k))
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, slsp.SampleCSV(k)); err != nil {
		log.Printf("could not write the template: %v", err)
	}
}

// expandHome turns a leading ~ into the user's home directory, so a path
// copied from a file manager or a terminal works either way.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
		}
	}
	return p
}

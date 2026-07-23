package semrush

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// analyticsPath is the base-relative prefix for backlinks reports. Domain,
// keyword, and url reports GET the reports base directly ("/"); backlinks
// reports GET <reports-base>/analytics/v1/.
const analyticsPath = "/analytics/v1/"

// defaultDisplayLimit caps report line count when the caller passes no --limit.
// Every returned line costs API units and Semrush's server default is 10,000
// lines/request, so a conservative default protects the unit balance; larger
// pulls are an explicit, deliberate --limit.
const defaultDisplayLimit = 10

// reportSpec declares one Semrush report as data. The factory (newReportCmd)
// turns it into a cobra command, so a new report is a table row, not new
// control flow.
type reportSpec struct {
	use       string // subcommand word, e.g. "overview"
	short     string // one-line help
	typ       string // Semrush type= value for the default form
	allDBTyp  string // type= when --all-databases is set (empty = flag unsupported)
	altTyp    string // type= when --paid is set (empty = flag unsupported)
	subject   string // query param the positional arg fills: domain|phrase|url|target
	argName   string // help name for the positional arg, e.g. "<domain>"
	joinArg   bool   // join multiple positional args with ';' into one subject value
	backlinks bool   // report lives under /analytics/v1/ and takes --target-type
}

// newReportCmd builds one report command from its spec. All reports share the
// same flag set; only the flags meaningful to a given report are emitted into
// the query.
func (s *Service) newReportCmd(key string, spec reportSpec) *cobra.Command {
	var flags reportFlags
	cmd := &cobra.Command{
		Use:         spec.use + " " + spec.argName,
		Short:       spec.short,
		Args:        cobra.ArbitraryArgs,
		Annotations: readOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			return s.runReport(cmd, key, spec, args, flags)
		},
	}
	registerReportFlags(cmd, &flags, spec)
	return cmd
}

// reportFlags holds every shared report flag. A report emits only the subset it
// supports; unset optional flags are omitted from the query.
type reportFlags struct {
	database     string
	allDatabases bool
	paid         bool
	limit        int
	offset       int
	columns      string
	filter       string
	sort         string
	date         string
	positions    string
	targetType   string
}

func registerReportFlags(cmd *cobra.Command, f *reportFlags, spec reportSpec) {
	fl := cmd.Flags()
	if !spec.backlinks {
		fl.StringVar(&f.database, "database", "us", "regional database, e.g. us|uk|de")
	}
	if spec.allDBTyp != "" {
		fl.BoolVar(&f.allDatabases, "all-databases", false, "aggregate across all regional databases")
	}
	if spec.altTyp != "" {
		fl.BoolVar(&f.paid, "paid", false, "paid/advertising variant of this report")
	}
	fl.IntVar(&f.limit, "limit", defaultDisplayLimit, "max rows (display_limit); every row costs API units")
	fl.IntVar(&f.offset, "offset", 0, "row offset (display_offset)")
	fl.StringVar(&f.columns, "columns", "", "comma-separated export_columns override")
	fl.StringVar(&f.filter, "filter", "", "display_filter expression")
	fl.StringVar(&f.sort, "sort", "", "display_sort expression, e.g. tr_desc")
	fl.StringVar(&f.date, "date", "", "display_date snapshot, e.g. 20260115")
	fl.StringVar(&f.positions, "positions", "", "display_positions: new|lost|rise|fall")
	if spec.backlinks {
		fl.StringVar(&f.targetType, "target-type", "root_domain", "target_type: root_domain|domain|url")
	}
}

// runReport builds the query for one report, fetches it, and emits parsed JSON.
func (s *Service) runReport(cmd *cobra.Command, key string, spec reportSpec, args []string, f reportFlags) error {
	subject, err := reportSubject(spec, args)
	if err != nil {
		return err
	}
	reportType := spec.typ
	if spec.allDBTyp != "" && f.allDatabases {
		reportType = spec.allDBTyp
	}
	if spec.altTyp != "" && f.paid {
		reportType = spec.altTyp
	}

	q := url.Values{}
	q.Set("type", reportType)
	q.Set(spec.subject, subject)
	q.Set("display_limit", strconv.Itoa(f.limit))
	// database applies only to single-database reports: the all-databases form
	// and backlinks reports are global and reject a database param.
	if !spec.backlinks && !(spec.allDBTyp != "" && f.allDatabases) {
		q.Set("database", f.database)
	}
	if spec.backlinks {
		q.Set("target_type", f.targetType)
	}
	setIfNonZero(q, "display_offset", f.offset)
	setIfNonEmpty(q, "export_columns", f.columns)
	setIfNonEmpty(q, "display_filter", f.filter)
	setIfNonEmpty(q, "display_sort", f.sort)
	setIfNonEmpty(q, "display_date", f.date)
	setIfNonEmpty(q, "display_positions", f.positions)

	root := strings.TrimRight(s.reportsBaseURL(), "/")
	base := root + "/"
	if spec.backlinks {
		base = root + analyticsPath
	}
	body, err := s.getRaw(cmd.Context(), base, q, key)
	if err != nil {
		return err
	}
	return s.emitReport(reportType, f, spec, string(body))
}

// reportSubject resolves the positional argument(s) into the single subject
// value. Most reports take exactly one; joinArg reports (keyword batch /
// difficulty) accept several joined by ';'.
func reportSubject(spec reportSpec, args []string) (string, error) {
	if len(args) == 0 {
		return "", &usageError{msg: fmt.Sprintf("missing required argument %s", spec.argName)}
	}
	if spec.joinArg {
		return strings.Join(args, ";"), nil
	}
	if len(args) > 1 {
		return "", &usageError{msg: fmt.Sprintf("expected a single %s argument, got %d", spec.argName, len(args))}
	}
	return args[0], nil
}

// emitReport renders one report body as JSON. An ERROR body is classified: a
// "nothing found" answer is a successful empty result (exit 0); any other ERROR
// becomes a runtime failure.
func (s *Service) emitReport(reportType string, f reportFlags, spec reportSpec, body string) error {
	if code, message, ok := parseSemrushError(body); ok {
		if code == nothingFoundCode {
			return s.emitJSON(reportEnvelope{
				Report:   reportType,
				Database: reportDatabase(f, spec),
				RowCount: 0,
				Rows:     []map[string]any{},
				Note:     "no data found for this query",
			})
		}
		return classifyReportError(code, message)
	}
	_, rows := parseCSVRows(body)
	return s.emitJSON(reportEnvelope{
		Report:   reportType,
		Database: reportDatabase(f, spec),
		RowCount: len(rows),
		Rows:     rows,
	})
}

// reportDatabase returns the database label for the envelope: empty for
// all-databases and backlinks (global) reports, otherwise the requested db.
func reportDatabase(f reportFlags, spec reportSpec) string {
	if spec.backlinks || (spec.allDBTyp != "" && f.allDatabases) {
		return ""
	}
	return f.database
}

// reportEnvelope is the provider-neutral JSON shape every report emits. Database
// is omitted when empty (global reports).
type reportEnvelope struct {
	Report   string           `json:"report"`
	Database string           `json:"database,omitempty"`
	RowCount int              `json:"row_count"`
	Rows     []map[string]any `json:"rows"`
	Note     string           `json:"note,omitempty"`
}

func (s *Service) emitJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return &apiError{msg: fmt.Sprintf("semrush: encode output: %v", err), err: err}
	}
	_, err = s.stdout().Write(append(b, '\n'))
	return err
}

func setIfNonEmpty(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

func setIfNonZero(q url.Values, key string, value int) {
	if value != 0 {
		q.Set(key, strconv.Itoa(value))
	}
}

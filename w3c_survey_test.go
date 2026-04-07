package goxslt

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/speedata/goxml"
	"github.com/speedata/goxpath"
)

// TestW3CSurvey runs ALL test cases from ALL test sets using subprocess isolation
// and produces a comprehensive report. Skipped by default (takes ~90s).
// Run with: go test -v -run TestW3CSurvey -timeout 3600s ./...
func TestW3CSurvey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping W3C survey in short mode")
	}
	if os.Getenv("W3C_SURVEY") == "" {
		t.Skip("skipping W3C survey (set W3C_SURVEY=1 to run)")
	}
	if _, err := os.Stat(w3cTestSuiteDir); os.IsNotExist(err) {
		t.Skip("W3C XSLT test suite not found at", w3cTestSuiteDir)
	}

	// Compile test binary once for reuse.
	testBinary := filepath.Join(os.TempDir(), "goxslt_survey.test")
	buildCmd := exec.Command("go", "test", "-c", "-o", testBinary)
	buildCmd.Dir = "."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("compiling test binary: %v\n%s", err, out)
	}
	defer os.Remove(testBinary)

	// Find all test-set XML files.
	testSetFiles, err := filepath.Glob(filepath.Join(w3cTestSuiteDir, "tests", "*", "*", "_*-test-set.xml"))
	if err != nil {
		t.Fatal(err)
	}

	type setStats struct {
		Category string
		SetName  string
		Total    int
		Pass     int
		Fail     int
		Panic    int
		Skip     int
		Errors   map[string]int
	}

	var allStats []setStats

	for _, tsFile := range testSetFiles {
		rel, _ := filepath.Rel(filepath.Join(w3cTestSuiteDir, "tests"), tsFile)
		parts := strings.Split(rel, string(filepath.Separator))
		category, setName := "", ""
		if len(parts) >= 2 {
			category = parts[0]
			setName = parts[1]
		}

		// Run this test set using the pre-compiled test binary.
		cmd := exec.Command(testBinary, "-test.v", "-test.run", "TestW3CSurveyOneSet", "-test.timeout", "30s")
		cmd.Env = append(os.Environ(), "W3C_SURVEY_SET_FILE="+tsFile)
		cmd.Dir = "."
		output, _ := cmd.CombinedOutput()

		// Parse the JSON results line from the output.
		stats := setStats{
			Category: category,
			SetName:  setName,
			Errors:   make(map[string]int),
		}

		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(line, "SURVEY_RESULT:") {
				jsonData := strings.TrimPrefix(line, "SURVEY_RESULT:")
				var result struct {
					Total  int            `json:"total"`
					Pass   int            `json:"pass"`
					Fail   int            `json:"fail"`
					Panic  int            `json:"panic"`
					Skip   int            `json:"skip"`
					Errors map[string]int `json:"errors"`
				}
				if err := json.Unmarshal([]byte(jsonData), &result); err == nil {
					stats.Total = result.Total
					stats.Pass = result.Pass
					stats.Fail = result.Fail
					stats.Panic = result.Panic
					stats.Skip = result.Skip
					stats.Errors = result.Errors
				}
			}
		}

		// If we got no results (crash/timeout), count total from XML.
		if stats.Total == 0 {
			data, err := os.ReadFile(tsFile)
			if err == nil {
				var ts w3cTestSet
				if xml.Unmarshal(data, &ts) == nil {
					stats.Total = len(ts.TestCases)
					stats.Panic = stats.Total
					stats.Errors["process crash/timeout"] = stats.Total
				}
			}
		}

		allStats = append(allStats, stats)
		fmt.Printf("  %-10s %-30s pass=%d fail=%d panic=%d skip=%d total=%d\n",
			category, setName, stats.Pass, stats.Fail, stats.Panic, stats.Skip, stats.Total)
	}

	// Sort by category, then set name.
	sort.Slice(allStats, func(i, j int) bool {
		if allStats[i].Category != allStats[j].Category {
			return allStats[i].Category < allStats[j].Category
		}
		return allStats[i].SetName < allStats[j].SetName
	})

	// Print summary.
	totalAll, passAll, failAll, panicAll, skipAll := 0, 0, 0, 0, 0
	fmt.Println("\n=== W3C XSLT 3.0 TEST SURVEY ===")
	fmt.Println()
	fmt.Printf("%-10s %-30s %6s %6s %6s %6s %6s\n", "Category", "Test Set", "Total", "Pass", "Fail", "Panic", "Skip")
	fmt.Println(strings.Repeat("-", 80))

	for _, s := range allStats {
		fmt.Printf("%-10s %-30s %6d %6d %6d %6d %6d\n",
			s.Category, s.SetName, s.Total, s.Pass, s.Fail, s.Panic, s.Skip)
		totalAll += s.Total
		passAll += s.Pass
		failAll += s.Fail
		panicAll += s.Panic
		skipAll += s.Skip
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("%-10s %-30s %6d %6d %6d %6d %6d\n",
		"", "TOTAL", totalAll, passAll, failAll, panicAll, skipAll)
	if totalAll > 0 {
		fmt.Printf("\nPass rate: %.1f%% (%d/%d)\n", float64(passAll)/float64(totalAll)*100, passAll, totalAll)
	}

	// Top error categories.
	errorTotals := make(map[string]int)
	for _, s := range allStats {
		for errCat, count := range s.Errors {
			errorTotals[errCat] += count
		}
	}

	type errEntry struct {
		Cat   string
		Count int
	}
	var errList []errEntry
	for cat, count := range errorTotals {
		errList = append(errList, errEntry{cat, count})
	}
	sort.Slice(errList, func(i, j int) bool { return errList[i].Count > errList[j].Count })

	fmt.Println("\n=== TOP ERROR CATEGORIES ===")
	for i, e := range errList {
		if i >= 40 {
			break
		}
		fmt.Printf("  %4d  %s\n", e.Count, e.Cat)
	}

	// Sets with most progress.
	fmt.Println("\n=== SETS SORTED BY PASS COUNT ===")
	sort.Slice(allStats, func(i, j int) bool { return allStats[i].Pass > allStats[j].Pass })
	for _, s := range allStats {
		if s.Pass > 0 {
			fmt.Printf("  %s/%s: %d pass, %d fail, %d panic, %d skip (of %d)\n",
				s.Category, s.SetName, s.Pass, s.Fail, s.Panic, s.Skip, s.Total)
		}
	}

	// Sets with 0 pass.
	fmt.Println("\n=== SETS WITH 0 PASS ===")
	sort.Slice(allStats, func(i, j int) bool { return allStats[i].Total > allStats[j].Total })
	for _, s := range allStats {
		if s.Pass == 0 && s.Total > 0 {
			topErr := ""
			maxCount := 0
			for cat, count := range s.Errors {
				if count > maxCount {
					maxCount = count
					topErr = cat
				}
			}
			fmt.Printf("  %s/%s: %d tests, top error: %s (%d)\n",
				s.Category, s.SetName, s.Total, topErr, maxCount)
		}
	}
}

// TestW3CSurveyOneSet is called by TestW3CSurvey as a subprocess.
// It reads W3C_SURVEY_SET_FILE env var and runs all tests in that set.
func TestW3CSurveyOneSet(t *testing.T) {
	tsFile := os.Getenv("W3C_SURVEY_SET_FILE")
	if tsFile == "" {
		t.Skip("W3C_SURVEY_SET_FILE not set")
	}

	data, err := os.ReadFile(tsFile)
	if err != nil {
		t.Fatal(err)
	}

	var ts w3cTestSet
	if err := xml.Unmarshal(data, &ts); err != nil {
		t.Fatal(err)
	}

	testSetDir := filepath.Dir(tsFile)

	envMap := make(map[string]*w3cEnvironment)
	for i := range ts.Environments {
		envMap[ts.Environments[i].Name] = &ts.Environments[i]
	}

	total, pass, fail, panicCount, skip := 0, 0, 0, 0, 0
	errors := make(map[string]int)

	for _, tc := range ts.TestCases {
		total++
		result := runSurveyTestCaseWithTimeout(tc, envMap, testSetDir, 5*time.Second)
		switch result.Status {
		case "PASS":
			pass++
		case "FAIL":
			fail++
			errCat := categorizeError(result.Error)
			errors[errCat]++
		case "PANIC":
			panicCount++
			errCat := "PANIC: " + categorizeError(result.Error)
			errors[errCat]++
		case "SKIP":
			skip++
		}
	}

	// Output result as JSON on a special line.
	result := struct {
		Total  int            `json:"total"`
		Pass   int            `json:"pass"`
		Fail   int            `json:"fail"`
		Panic  int            `json:"panic"`
		Skip   int            `json:"skip"`
		Errors map[string]int `json:"errors"`
	}{total, pass, fail, panicCount, skip, errors}

	jsonData, _ := json.Marshal(result)
	fmt.Printf("SURVEY_RESULT:%s\n", jsonData)
}

type surveyResult struct {
	Status string
	Error  string
}

func runSurveyTestCaseWithTimeout(tc w3cTestCase, envMap map[string]*w3cEnvironment, baseDir string, timeout time.Duration) surveyResult {
	ch := make(chan surveyResult, 1)
	go func() {
		ch <- runSurveyTestCase(tc, envMap, baseDir)
	}()
	select {
	case res := <-ch:
		return res
	case <-time.After(timeout):
		return surveyResult{Status: "PANIC", Error: "timeout after " + timeout.String()}
	}
}

func runSurveyTestCase(tc w3cTestCase, envMap map[string]*w3cEnvironment, baseDir string) (result surveyResult) {
	defer func() {
		if r := recover(); r != nil {
			result = surveyResult{Status: "PANIC", Error: fmt.Sprintf("%v", r)}
		}
	}()

	expectError := w3cExpectsError(tc.Result)

	sourceXML, err := w3cResolveSource(tc, envMap, baseDir)
	if err != nil {
		sourceXML = "<empty/>"
	}

	sourceDoc, err := goxml.Parse(strings.NewReader(sourceXML))
	if err != nil {
		if expectError {
			return surveyResult{Status: "PASS"}
		}
		return surveyResult{Status: "FAIL", Error: "source parse: " + err.Error()}
	}

	var xsltPath string
	if len(tc.Test.Stylesheets) > 0 {
		xsltPath = filepath.Join(baseDir, tc.Test.Stylesheets[0].File)
	} else if tc.Environment.Ref != "" {
		if env, ok := envMap[tc.Environment.Ref]; ok && len(env.Stylesheets) > 0 {
			xsltPath = filepath.Join(baseDir, env.Stylesheets[0].File)
		}
	}
	if xsltPath == "" {
		return surveyResult{Status: "SKIP", Error: "no stylesheet"}
	}

	ss, err := CompileFile(xsltPath)
	if err != nil {
		if expectError {
			return surveyResult{Status: "PASS"}
		}
		return surveyResult{Status: "FAIL", Error: "compile: " + err.Error()}
	}

	opts := TransformOptions{}
	if tc.Test.InitialTemplate != nil {
		opts.InitialTemplate = tc.Test.InitialTemplate.Name
	}
	if len(tc.Test.Params) > 0 {
		opts.Parameters = make(map[string]goxpath.Sequence)
		for _, p := range tc.Test.Params {
			np := &goxpath.Parser{Ctx: goxpath.NewContext(sourceDoc)}
			seq, evalErr := np.Evaluate(p.Select)
			if evalErr != nil {
				return surveyResult{Status: "FAIL", Error: "param eval: " + evalErr.Error()}
			}
			opts.Parameters[p.Name] = seq
		}
	}

	resultDoc, err := TransformWithOptions(ss, sourceDoc, opts)
	if err != nil {
		if expectError {
			return surveyResult{Status: "PASS"}
		}
		return surveyResult{Status: "FAIL", Error: "transform: " + err.Error()}
	}

	if expectError {
		return surveyResult{Status: "FAIL", Error: "expected error but succeeded"}
	}

	got := SerializeWithOutput(resultDoc.Document, resultDoc.Output)

	ok, reason := surveyAssertResult(tc.Result, got, baseDir)
	if ok {
		return surveyResult{Status: "PASS"}
	}
	return surveyResult{Status: "FAIL", Error: reason}
}

func surveyAssertResult(result w3cResult, got string, baseDir string) (bool, string) {
	if len(result.AssertXML) > 0 {
		return surveyCheckAssertXML(result.AssertXML[0], got, baseDir)
	}

	if len(result.Assert) > 0 {
		return surveyCheckAssert(result.Assert[0], got)
	}

	if len(result.AssertStringValue) > 0 {
		return surveyCheckAssertStringValue(result.AssertStringValue[0], got)
	}

	if len(result.SerializationMatch) > 0 {
		return surveyCheckSerializationMatch(result.SerializationMatch[0], got)
	}

	if result.AllOf != nil && w3cAllOfHasAssertions(result.AllOf) {
		for _, ax := range result.AllOf.AssertXML {
			ok, reason := surveyCheckAssertXML(ax, got, baseDir)
			if !ok {
				return false, reason
			}
		}
		for _, a := range result.AllOf.Assert {
			ok, reason := surveyCheckAssert(a, got)
			if !ok {
				return false, reason
			}
		}
		for _, ao := range result.AllOf.AnyOf {
			ok, reason := surveyCheckAnyOf(ao, got, baseDir)
			if !ok {
				return false, reason
			}
		}
		for _, sv := range result.AllOf.AssertStringValue {
			ok, reason := surveyCheckAssertStringValue(sv, got)
			if !ok {
				return false, reason
			}
		}
		for _, sm := range result.AllOf.SerializationMatch {
			ok, reason := surveyCheckSerializationMatch(sm, got)
			if !ok {
				return false, reason
			}
		}
		return true, ""
	}

	if result.AnyOf != nil && w3cAnyOfHasAssertions(result.AnyOf) {
		return surveyCheckAnyOf(*result.AnyOf, got, baseDir)
	}

	return false, "unsupported assertion type"
}

func surveyCheckAssertStringValue(sv w3cAssertString, got string) (bool, string) {
	gotStr := w3cExtractStringValue(got)
	expected := sv.Value
	if sv.Normalize == "yes" {
		gotStr = strings.Join(strings.Fields(gotStr), " ")
		expected = strings.Join(strings.Fields(expected), " ")
	}
	if gotStr == expected {
		return true, ""
	}
	return false, "string value mismatch"
}

func surveyCheckSerializationMatch(sm w3cSerializationMatch, got string) (bool, string) {
	re, err := regexp.Compile(sm.Pattern)
	if err != nil {
		return false, "invalid regex: " + err.Error()
	}
	if re.MatchString(got) {
		return true, ""
	}
	return false, "serialization mismatch"
}

func surveyCheckAssertXML(ax w3cAssertXML, got string, baseDir string) (bool, string) {
	expected, err := w3cResolveAssertXML(ax, baseDir)
	if err != nil {
		return false, "resolve expected: " + err.Error()
	}
	gotBody := w3cNormalizeXML(w3cStripXMLDecl(got))
	expectedBody := w3cNormalizeXML(w3cStripXMLDecl(expected))
	if gotBody == expectedBody {
		return true, ""
	}
	return false, "xml mismatch"
}

func surveyCheckAssert(a w3cAssert, got string) (bool, string) {
	doc, err := goxml.Parse(strings.NewReader(w3cStripXMLDecl(got)))
	if err != nil {
		return false, "assert parse: " + err.Error()
	}
	np := &goxpath.Parser{Ctx: goxpath.NewContext(doc)}
	np.Ctx.SetContextSequence(goxpath.Sequence{doc})
	np.Ctx.Namespaces["j"] = "http://www.w3.org/2005/xpath-functions"
	seq, err := np.Evaluate(a.XPath)
	if err != nil {
		return false, "assert eval: " + err.Error()
	}
	boolVal, err := goxpath.BooleanValue(seq)
	if err != nil {
		return false, "assert bool: " + err.Error()
	}
	if boolVal {
		return true, ""
	}
	return false, "assert false: " + a.XPath
}

func surveyCheckAnyOf(ao w3cAnyOf, got string, baseDir string) (bool, string) {
	for _, ax := range ao.AssertXML {
		expected, err := w3cResolveAssertXML(ax, baseDir)
		if err != nil {
			continue
		}
		if w3cNormalizeXML(w3cStripXMLDecl(got)) == w3cNormalizeXML(w3cStripXMLDecl(expected)) {
			return true, ""
		}
	}
	for _, a := range ao.Assert {
		doc, err := goxml.Parse(strings.NewReader(w3cStripXMLDecl(got)))
		if err != nil {
			continue
		}
		np := &goxpath.Parser{Ctx: goxpath.NewContext(doc)}
		np.Ctx.SetContextSequence(goxpath.Sequence{doc})
		np.Ctx.Namespaces["j"] = "http://www.w3.org/2005/xpath-functions"
		seq, err := np.Evaluate(a.XPath)
		if err != nil {
			continue
		}
		boolVal, err := goxpath.BooleanValue(seq)
		if err != nil {
			continue
		}
		if boolVal {
			return true, ""
		}
	}
	for _, sv := range ao.AssertStringValue {
		gotStr := w3cExtractStringValue(got)
		expected := sv.Value
		if sv.Normalize == "yes" {
			gotStr = strings.Join(strings.Fields(gotStr), " ")
			expected = strings.Join(strings.Fields(expected), " ")
		}
		if gotStr == expected {
			return true, ""
		}
	}
	for _, sm := range ao.SerializationMatch {
		re, err := regexp.Compile(sm.Pattern)
		if err != nil {
			continue
		}
		if re.MatchString(got) {
			return true, ""
		}
	}
	if len(ao.Error) > 0 {
		return true, ""
	}
	return false, "any-of: no match"
}

func categorizeError(errMsg string) string {
	if strings.Contains(errMsg, "unsupported assertion") {
		return "unsupported assertion type"
	}
	if strings.Contains(errMsg, "expected error but succeeded") {
		return "expected error but succeeded"
	}
	if strings.Contains(errMsg, "no stylesheet") {
		return "no stylesheet"
	}
	if strings.Contains(errMsg, "timeout after") {
		return "timeout (infinite recursion/hang)"
	}

	if strings.HasPrefix(errMsg, "compile:") {
		msg := strings.TrimPrefix(errMsg, "compile: ")
		if strings.Contains(msg, "unsupported XSLT element") {
			if idx := strings.Index(msg, "unsupported XSLT element"); idx >= 0 {
				return "compile: " + msg[idx:]
			}
		}
		if strings.Contains(msg, "unknown instruction") {
			if idx := strings.Index(msg, "unknown instruction"); idx >= 0 {
				rest := msg[idx:]
				if nl := strings.Index(rest, "\n"); nl > 0 {
					rest = rest[:nl]
				}
				return "compile: " + rest
			}
		}
		if len(msg) > 100 {
			msg = msg[:100]
		}
		return "compile: " + msg
	}

	if strings.HasPrefix(errMsg, "transform:") {
		msg := strings.TrimPrefix(errMsg, "transform: ")
		if len(msg) > 100 {
			msg = msg[:100]
		}
		return "transform: " + msg
	}

	if strings.HasPrefix(errMsg, "source parse:") {
		return "source parse error"
	}
	if strings.HasPrefix(errMsg, "xml mismatch") {
		return "xml mismatch (wrong output)"
	}
	if strings.HasPrefix(errMsg, "assert false:") {
		return "assertion failed"
	}
	if strings.HasPrefix(errMsg, "assert eval:") || strings.HasPrefix(errMsg, "assert parse:") {
		return "assertion evaluation error"
	}
	if strings.HasPrefix(errMsg, "any-of:") {
		return "any-of: no match"
	}
	if strings.HasPrefix(errMsg, "param eval:") {
		return "parameter evaluation error"
	}

	if len(errMsg) > 100 {
		errMsg = errMsg[:100]
	}
	return errMsg
}

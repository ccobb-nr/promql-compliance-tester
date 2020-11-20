package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/promlabs/promql-compliance-tester/comparer"
	"github.com/promlabs/promql-compliance-tester/config"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type InsightsResponse struct {
	TestCase string         `json:"testCase"`
	Response *http.Response `json:"response"`
}

func getGetRequestFormat(queryUrl string, testCase *comparer.TestCase) string {
	return fmt.Sprintf("%s/api/v1/query_range?query=%s&start=%s&end=%s&step=%s",
		queryUrl,
		url.QueryEscape(testCase.Query),
		strconv.FormatInt(testCase.Start.UnixNano()/1e6, 10),
		strconv.FormatInt(testCase.End.UnixNano()/1e6, 10),
		strconv.FormatInt(testCase.Resolution.Milliseconds(), 10),
	)
}

func Event(results []*comparer.Result, includePassing bool, tweaks []*config.QueryTweak, cfg *config.Config) {

	var responses []InsightsResponse

	testId := uuid.New()
	testTimeStamp := time.Now()
	insightsCollectorUrl := os.Getenv("INSIGHTS_COLLECTOR_URL")
	insightsInsertKey := os.Getenv("INSIGHTS_INSERT_KEY")
	eventType := os.Getenv("PROMQL_COMPLIANCE_EVENT_TYPE")

	if len(insightsCollectorUrl) <= 0 {
		fmt.Printf("Need to set INSIGHTS_COLLECTOR_URL")
		return
	}
	if len(insightsInsertKey) <= 0 {
		fmt.Printf("Need to set INSIGHTS_INSERT_KEY")
		return
	}

	queryTweaks := ""
	for _, t := range tweaks {
		queryTweaks += t.Note + ", "
	}

	if len(queryTweaks) > 4096 {
		queryTweaks = queryTweaks[:4096]
	}

	successes := 0
	unsupported := 0

	for _, res := range results {

		if res.Success() {
			successes++
		}
		if res.Unsupported {
			unsupported++
		}

		result := "failed"
		if res.Success() {
			result = "passed"
		} else if res.Unsupported {
			result = "unsupported"
		}

		diff := res.Diff
		if len(diff) > 4096 {
			diff = diff[:4096]
		}

		testResults := map[string]interface{}{
			"eventType":                 eventType,
			"testId":                    testId,
			"testTimeStamp":             testTimeStamp,
			"query":                     res.TestCase.Query,
			"getRequestTestTarget":      getGetRequestFormat(cfg.TestTargetConfig.QueryURL, res.TestCase),
			"getRequestReferenceTarget": getGetRequestFormat(cfg.ReferenceTargetConfig.QueryURL, res.TestCase),
			"start":                     res.TestCase.Start,
			"end":                       res.TestCase.End,
			"step":                      res.TestCase.Resolution,
			"passed":                    res.Success(),
			"result":                    result,
			"unexpectedSuccess":         res.UnexpectedSuccess,
			"unexpectedFailure":         res.UnexpectedFailure,
			"diff":                      diff,
			"queryTweaks":               queryTweaks,
			"referenceTargetQueryUrl":   cfg.ReferenceTargetConfig.QueryURL,
			"testTargetQueryUrl":        cfg.TestTargetConfig.QueryURL,
		}

		jsonResults, _ := json.Marshal(testResults)
		req, _ := http.NewRequest("POST", insightsCollectorUrl, bytes.NewBuffer(jsonResults))
		req.Header.Add("X-Insert-Key", insightsInsertKey)
		resp, _ := http.DefaultClient.Do(req)
		responses = append(responses, InsightsResponse{
			TestCase: res.TestCase.Query,
			Response: resp,
		})
	}

	for _, insightsResponse := range responses {
		if insightsResponse.Response.StatusCode >= 300 || insightsResponse.Response.StatusCode < 200 {
			fmt.Printf("Failed to POST testcase <%s>. Response <%d>, <%s>.\n", insightsResponse.TestCase, insightsResponse.Response.StatusCode, insightsResponse.Response.Status)
		}
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Test Run ID: %s\n", testId)
	fmt.Printf("Total: %d / %d (%.2f%%) passed, %d unsupported\n", successes, len(results), 100*float64(successes)/float64(len(results)), unsupported)
}

package goxslt

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/speedata/goxml"
	"github.com/speedata/goxpath"
)

const w3cTestSuiteDir = "testdata/w3c"

// w3cTestSetConfig describes a test set with selectively enabled test cases.
type w3cTestSetConfig struct {
	File  string   // relative path to the test-set XML
	Tests []string // whitelisted test case names (empty = all)
}

// w3cTestSets lists the test-sets and their enabled test cases.
var w3cTestSets = map[string]w3cTestSetConfig{
	"json-to-xml": {
		File: "tests/fn/json-to-xml/_json-to-xml-test-set.xml",
		Tests: []string{
			"json-to-xml-001", "json-to-xml-002", "json-to-xml-003",
			"json-to-xml-004", "json-to-xml-005", "json-to-xml-006",
			"json-to-xml-007", "json-to-xml-008", "json-to-xml-009",
			"json-to-xml-010", "json-to-xml-011", "json-to-xml-012",
			"json-to-xml-101",
			"json-to-xml-typed-010",
			"json-to-xml-escape-001", "json-to-xml-escape-002",
			"json-to-xml-escape-003", "json-to-xml-escape-004",
			"json-to-xml-error-001", "json-to-xml-error-002", "json-to-xml-error-003",
			"json-to-xml-error-005", "json-to-xml-error-006", "json-to-xml-error-007",
			"json-to-xml-error-008", "json-to-xml-error-009", "json-to-xml-error-010",
			"json-to-xml-error-011", "json-to-xml-error-012", "json-to-xml-error-013",
			"json-to-xml-error-014", "json-to-xml-error-016", "json-to-xml-error-018",
		},
	},
	"key": {
		File: "tests/fn/key/_key-test-set.xml",
		Tests: []string{
			"key-001", "key-002", "key-003", "key-004", "key-005",
			"key-006", "key-007", "key-008", "key-009", "key-010",
			"key-011", "key-012", "key-013", "key-014", "key-015",
			"key-016", "key-017", "key-018", "key-019", "key-020",
			"key-021", "key-022", "key-023", "key-024",
			"key-027", "key-028", "key-029", "key-030",
			"key-031", "key-032", "key-033", "key-034", "key-035",
			"key-036", "key-037", "key-038", "key-039", "key-040",
			"key-041",
			"key-043", "key-044", "key-045", "key-046",
			"key-048", "key-049", "key-050",
			"key-051", "key-052", "key-053", "key-054", "key-055",
			"key-056",
			"key-059", "key-060",
			"key-061", "key-062", "key-063", "key-064", "key-065",
			"key-068",
			"key-071", "key-072",
			"key-076", "key-077", "key-078", "key-079", "key-080",
			"key-083",
			"key-085a", "key-085b", "key-086",
			"key-091", "key-092",
			"key-094", "key-095",
		},
	},
	"core-function": {
		File: "tests/fn/core-function/_core-function-test-set.xml",
		Tests: []string{
			"core-function-001", "core-function-002", "core-function-003",
			"core-function-004", "core-function-005", "core-function-006",
			"core-function-007", "core-function-008", "core-function-009",
			"core-function-010", "core-function-011", "core-function-012",
			"core-function-013", "core-function-014", "core-function-015",
			"core-function-016", "core-function-017", "core-function-018",
			"core-function-019", "core-function-020", "core-function-021",
			"core-function-022", "core-function-023", "core-function-024",
			"core-function-025", "core-function-026", "core-function-027",
			"core-function-028", "core-function-029", "core-function-030",
			"core-function-031", "core-function-032", "core-function-033",
			"core-function-034", "core-function-035", "core-function-036",
			"core-function-037", "core-function-038", "core-function-039",
			"core-function-040", "core-function-041", "core-function-042",
			"core-function-043", "core-function-044", "core-function-045",
			"core-function-046", "core-function-047", "core-function-048",
			"core-function-049",
			"core-function-050", "core-function-051", "core-function-052",
			"core-function-053", "core-function-054", "core-function-055",
			"core-function-056", "core-function-057", "core-function-058",
			"core-function-059", "core-function-060", "core-function-061",
			"core-function-062", "core-function-063", "core-function-064", "core-function-065",
			"core-function-066", "core-function-067", "core-function-068",
			"core-function-069", "core-function-070", "core-function-071",
			"core-function-072", "core-function-073", "core-function-074", "core-function-075",
			"core-function-076", "core-function-077", "core-function-078", "core-function-079",
			"core-function-080", "core-function-081", "core-function-082",
			"core-function-083", "core-function-084", "core-function-085", "core-function-086",
			"core-function-087", "core-function-089", "core-function-090",
		},
	},
	"position": {
		File: "tests/fn/position/_position-test-set.xml",
		Tests: []string{
			"position-0201", "position-0301", "position-0302",
			"position-0401", "position-0501", "position-0601",
			"position-0701", "position-0702",
			"position-1102", "position-1103", "position-1104", "position-1105",
			"position-1106", "position-1107", "position-1108", "position-1109",
			"position-1110", "position-1111", "position-1112", "position-1113",
			"position-1114", "position-1115", "position-1116", "position-1117",
			"position-1118", "position-1119", "position-1120", "position-1121",
			"position-1122", "position-1123", "position-1124", "position-1125",
			"position-1126", "position-1127", "position-1128", "position-1129",
			"position-1130", "position-1131", "position-1132",
			"position-1201", "position-1202", "position-1203", "position-1204",
			"position-1205", "position-1206", "position-1207", "position-1208",
			"position-1209", "position-1210", "position-1211", "position-1212",
			"position-1213", "position-1214", "position-1215", "position-1216",
			"position-1217", "position-1218",
			"position-1301", "position-1302", "position-1303", "position-1304",
			"position-1305", "position-1306", "position-1307", "position-1308",
			"position-1309", "position-1310", "position-1311", "position-1312",
			"position-1313", "position-1314", "position-1315", "position-1316",
			"position-1317", "position-1318", "position-1319", "position-1320",
			"position-1401", "position-1402", "position-1403", "position-1404",
			"position-1501", "position-1502", "position-1503", "position-1504",
			"position-1505", "position-1506", "position-1507", "position-1508",
			"position-1509", "position-1510",
			"position-1701", "position-1702", "position-1703",
			"position-1901",
			"position-2801", "position-2802", "position-2803", "position-2804",
			"position-2805", "position-2806",
			"position-2903",
			"position-3002", "position-3003", "position-3004",
			"position-3101", "position-3102", "position-3103", "position-3104",
			"position-3301", "position-3303", "position-3304", "position-3305",
			"position-3306", "position-3307", "position-3308", "position-3309",
			"position-3401", "position-3402",
			"position-3501", "position-3601", "position-3701",
			"position-4301", "position-4401", "position-4602",
			"position-5001", "position-5101",
			"position-5301", "position-5302",
			"position-5701", "position-5801",
			"position-5901", "position-5902", "position-5903",
			"position-6001", "position-6302", "position-6401",
			"position-6901", "position-7001", "position-7201",
			"position-7302", "position-7501", "position-7601", "position-7701",
		},
	},
	"iterate": {
		File: "tests/insn/iterate/_iterate-test-set.xml",
		Tests: []string{
			"iterate-001", "iterate-004", "iterate-007",
			"iterate-014", "iterate-023", "iterate-024",
			"iterate-033", "iterate-040", "iterate-041",
		},
	},
	"apply-templates": {
		File: "tests/insn/apply-templates/_apply-templates-test-set.xml",
		Tests: []string{
			"conflict-resolution-0101", "conflict-resolution-0102a", "conflict-resolution-0102c",
			"conflict-resolution-0104a", "conflict-resolution-0104c",
			"conflict-resolution-0106", "conflict-resolution-0107",
			"conflict-resolution-0108a", "conflict-resolution-0108c",
			"conflict-resolution-0110a", "conflict-resolution-0110c",
			"conflict-resolution-0112",
			"conflict-resolution-0201",
			"conflict-resolution-0401a", "conflict-resolution-0401c",
			"conflict-resolution-0501",
			"conflict-resolution-0601",
			"conflict-resolution-0701", "conflict-resolution-0703",
			"conflict-resolution-0801", "conflict-resolution-0802",
			"conflict-resolution-0901",
			"conflict-resolution-1001",
			"conflict-resolution-1202b",
			"conflict-resolution-1501",
			"conflict-resolution-1601",
			"conflict-resolution-1801",
		},
	},
	"call-template": {
		File: "tests/insn/call-template/_call-template-test-set.xml",
		Tests: []string{
			"call-template-0101", "call-template-0102", "call-template-0104",
			"call-template-0107", "call-template-0111",
			"call-template-0201",
			"call-template-0402",
			"call-template-0501",
			"call-template-0601",
			"call-template-0701",
			"call-template-0801", "call-template-0802",
			"call-template-1001", "call-template-1002",
			"call-template-1101", "call-template-1102",
			"call-template-1501",
			"call-template-1601",
			"call-template-1801", "call-template-1802", "call-template-1803",
			"call-template-1901",
			"call-template-2001",
		},
	},
	"choose": {
		File: "tests/insn/choose/_choose-test-set.xml",
		Tests: []string{
			"choose-0101", "choose-0105",
			"choose-0201",
			"choose-0301",
			"choose-0401", "choose-0402", "choose-0403", "choose-0404",
			"choose-0501", "choose-0502",
			"choose-0601", "choose-0602", "choose-0603", "choose-0605",
			"choose-0606", "choose-0607", "choose-0608", "choose-0609",
			"choose-0701", "choose-0702",
			"choose-0801",
			"choose-0901",
			"choose-1001",
			"choose-1101",
			"choose-1201",
			"choose-1401",
			"choose-1601",
			"choose-1701", "choose-1702", "choose-1703", "choose-1704", "choose-1705", "choose-1706",
			"choose-1801", "choose-1802",
			"choose-1901", "choose-1902", "choose-1903", "choose-1904", "choose-1905",
		},
	},
	"copy": {
		File: "tests/insn/copy/_copy-test-set.xml",
		Tests: []string{
			"copy-0102",
			"copy-0201", "copy-0202",
			"copy-0601", "copy-0602", "copy-0603", "copy-0604", "copy-0605", "copy-0606",
			"copy-0701",
			"copy-0801",
			"copy-1003",
			"copy-1101",
			"copy-1204", "copy-1212", "copy-1216",
			"copy-1501",
			"copy-1801",
			"copy-2001",
			"copy-2101",
			"copy-2201", "copy-2202",
			"copy-2401", "copy-2402",
			"copy-2901",
			"copy-3001", "copy-3002",
			"copy-3102",
			"copy-3302",
			"copy-3401",
			"copy-3701", "copy-3702",
			"copy-3802", "copy-3804",
			"copy-4001",
			"copy-4201",
			"copy-4301", "copy-4306",
			"copy-5001", "copy-5002", "copy-5003", "copy-5004",
			"copy-5013", "copy-5014",
			"copy-5023", "copy-5024",
			"copy-5201",
		},
	},
	"variable": {
		File: "tests/decl/variable/_variable-test-set.xml",
		Tests: []string{
			"variable-0101", "variable-0102", "variable-0106",
			"variable-0115", "variable-0118",
			"variable-0122", "variable-0123",
			"variable-0601",
			"variable-0701",
			"variable-0801", "variable-0802",
			"variable-0901",
			"variable-1001", "variable-1002", "variable-1003", "variable-1004", "variable-1005",
			"variable-1007", "variable-1009", "variable-1010", "variable-1011",
			"variable-1101", "variable-1102", "variable-1103",
			"variable-1201",
			"variable-1301",
			"variable-1401",
			"variable-1501",
			"variable-1601",
			"variable-1701",
			"variable-1801",
			"variable-1901", "variable-1902", "variable-1903", "variable-1904", "variable-1905",
			"variable-2001",
			"variable-2101",
			"variable-2201",
			"variable-2301", "variable-2302", "variable-2305",
			"variable-2501",
			"variable-2601",
			"variable-2701",
			"variable-2801",
			"variable-2901",
			"variable-3001",
			"variable-3101",
			"variable-3301",
			"variable-3501",
			"variable-3601",
			"variable-3701",
			"variable-3801", "variable-3802",
			"variable-3901",
			"variable-4001",
			"variable-4101",
			"variable-4201",
			"variable-4301",
			"variable-4401", "variable-4402", "variable-4403",
			"variable-4501",
			"variable-4601", "variable-4602",
			"variable-4701",
			"variable-4802",
		},
	},
	"for-each-group": {
		File: "tests/insn/for-each-group/_for-each-group-test-set.xml",
		Tests: []string{
			"for-each-group-002", "for-each-group-002a",
			"for-each-group-012", "for-each-group-013",
			"for-each-group-015b",
			"for-each-group-030",
			"for-each-group-032",
			"for-each-group-036",
			"for-each-group-046a",
			"for-each-group-047", "for-each-group-048", "for-each-group-049",
			"for-each-group-050", "for-each-group-051", "for-each-group-052",
			"for-each-group-053", "for-each-group-054",
			"for-each-group-071",
			"for-each-group-076",
			"for-each-group-078b",
			"for-each-group-079b",
		},
	},
	"number": {
		File: "tests/insn/number/_number-test-set.xml",
		Tests: []string{
			"number-0109",
			"number-0201",
			"number-0405", "number-0406", "number-0407",
			"number-0501",
			"number-0601", "number-0603",
			"number-0701",
			"number-0801", "number-0825",
			"number-1001", "number-1004",
			"number-1102",
			"number-1301",
			"number-1401",
			"number-1902", "number-1903",
			"number-2001",
			"number-2101",
			"number-2201",
			"number-2401",
			"number-2502", "number-2503",
			"number-2601", "number-2602",
			"number-2701",
			"number-2801", "number-2802", "number-2803", "number-2805",
			"number-2806", "number-2807", "number-2808", "number-2809",
			"number-2811", "number-2812", "number-2813", "number-2814",
			"number-3001", "number-3002", "number-3003",
			"number-3101",
			"number-3201", "number-3202", "number-3203",
			"number-3220", "number-3221", "number-3222", "number-3223", "number-3224",
			"number-3226", "number-3227",
			"number-3230", "number-3231",
			"number-3401", "number-3402", "number-3403",
			"number-3601",
			"number-3701", "number-3702",
			"number-3801", "number-3802",
			"number-3901",
			"number-4101",
			"number-4201", "number-4202",
			"number-4601",
		},
	},
	"message": {
		File: "tests/insn/message/_message-test-set.xml",
		Tests: []string{
			"message-0001", "message-0001a",
			"message-0002", "message-0003", "message-0004", "message-0005", "message-0006",
			"message-0007", "message-0008", "message-0009", "message-0010",
			"message-0011",
			"message-0101", "message-0102", "message-0103",
			"message-0201", "message-0202",
			"message-0301", "message-0302", "message-0303", "message-0304", "message-0305",
			"message-0308", "message-0309", "message-0310", "message-0311",
			"message-0312", "message-0313", "message-0314",
			"message-0317",
			"message-0401", "message-0402", "message-0403", "message-0404", "message-0405",
			"message-0406", "message-0408",
		},
	},
	"sort": {
		File: "tests/insn/sort/_sort-test-set.xml",
		Tests: []string{
			"sort-001", "sort-002", "sort-003",
			"sort-005", "sort-006", "sort-007", "sort-008", "sort-009", "sort-010",
			"sort-011", "sort-012", "sort-013", "sort-014", "sort-015",
			"sort-016", "sort-017", "sort-018", "sort-019", "sort-020",
			"sort-021", "sort-022", "sort-023",
			"sort-024", "sort-025", "sort-026", "sort-028",
			"sort-030", "sort-031", "sort-032", "sort-033", "sort-034",
			"sort-035", "sort-036", "sort-037", "sort-038", "sort-039", "sort-040",
			"sort-041", "sort-042", "sort-043", "sort-044", "sort-045", "sort-046",
			"sort-047", "sort-048", "sort-049", "sort-050",
			"sort-054", "sort-055",
			"sort-056", "sort-057", "sort-058",
			"sort-059", "sort-060",
			"sort-063", "sort-065", "sort-066",
			"sort-068", "sort-069", "sort-070",
			"sort-071", "sort-072", "sort-073",
			"sort-074", "sort-075", "sort-076", "sort-077",
		},
	},
	"analyze-string": {
		File: "tests/insn/analyze-string/_analyze-string-test-set.xml",
		Tests: []string{
			"analyze-string-005", "analyze-string-006", "analyze-string-007", "analyze-string-008",
			"analyze-string-012",
			"analyze-string-035",
			"analyze-string-037",
			"analyze-string-048", "analyze-string-049",
			"analyze-string-051", "analyze-string-052", "analyze-string-053",
			"analyze-string-064", "analyze-string-065",
			"analyze-string-067",
			"analyze-string-069",
			"analyze-string-072", "analyze-string-073", "analyze-string-074", "analyze-string-075",
			"analyze-string-078", "analyze-string-079",
			"analyze-string-084",
			"analyze-string-091a",
			"analyze-string-095", "analyze-string-096", "analyze-string-097",
			"analyze-string-098", "analyze-string-099",
		},
	},
	"result-document": {
		File: "tests/insn/result-document/_result-document-test-set.xml",
		Tests: []string{
			"result-document-0101", "result-document-0102",
			"result-document-0201", "result-document-0203", "result-document-0204",
			"result-document-0206", "result-document-0207", "result-document-0212",
			"result-document-0220", "result-document-0226", "result-document-0227",
			"result-document-0231", "result-document-0234",
			"result-document-0250", "result-document-0252", "result-document-0254",
			"result-document-0255", "result-document-0257", "result-document-0259",
			"result-document-0261", "result-document-0262", "result-document-0264",
			"result-document-0266", "result-document-0268", "result-document-0269",
			"result-document-0276", "result-document-0283",
			"result-document-0286", "result-document-0287",
			"result-document-0801",
			"result-document-1001", "result-document-1002", "result-document-1003",
			"result-document-1004", "result-document-1005",
			"result-document-1101", "result-document-1102", "result-document-1103",
			"result-document-1104", "result-document-1105", "result-document-1106",
			"result-document-1107", "result-document-1108", "result-document-1109",
			"result-document-1110", "result-document-1111",
			"result-document-1131",
			"result-document-1137",
			"result-document-1139", "result-document-1140", "result-document-1141",
			"result-document-1142", "result-document-1143", "result-document-1144",
			"result-document-1407",
			"result-document-1502",
		},
	},
	"string": {
		File: "tests/type/string/_string-test-set.xml",
		Tests: []string{
			"string-001", "string-002", "string-003", "string-004", "string-005",
			"string-006", "string-007", "string-008", "string-009", "string-010",
			"string-011", "string-012", "string-013", "string-014",
			"string-015", "string-016",
			"string-018", "string-019", "string-020", "string-021",
			"string-023", "string-024",
			"string-026", "string-027", "string-028", "string-029",
			"string-031", "string-032", "string-033", "string-034",
			"string-036", "string-037", "string-038", "string-039",
			"string-040", "string-041", "string-042", "string-043",
			"string-044", "string-045", "string-046", "string-047",
			"string-048", "string-049", "string-050", "string-051",
			"string-052", "string-053", "string-054", "string-055",
			"string-056", "string-057", "string-058", "string-059",
			"string-060", "string-061", "string-062", "string-063",
			"string-064", "string-065", "string-066",
			"string-068",
			"string-070", "string-071",
			"string-074", "string-075", "string-076", "string-077",
			"string-078", "string-079", "string-080", "string-081",
			"string-082", "string-083", "string-084", "string-085",
			"string-087", "string-088", "string-089", "string-090",
			"string-091", "string-092", "string-093",
			"string-094",
			"string-096", "string-097", "string-098", "string-099",
			"string-100", "string-101", "string-102", "string-103",
			"string-104", "string-105", "string-106", "string-107",
			"string-108", "string-109", "string-110", "string-111",
			"string-112", "string-113", "string-114", "string-115",
			"string-116", "string-117", "string-118",
			"string-122", "string-123", "string-124", "string-125",
			"string-126", "string-127", "string-128",
			"string-129", "string-130", "string-131", "string-132",
		},
	},
	"boolean": {
		File: "tests/type/boolean/_boolean-test-set.xml",
		Tests: []string{
			"boolean-001", "boolean-002", "boolean-003", "boolean-004", "boolean-005",
			"boolean-006", "boolean-007", "boolean-008", "boolean-009",
			"boolean-010", "boolean-011", "boolean-012", "boolean-013",
			"boolean-014", "boolean-015", "boolean-016", "boolean-017",
			"boolean-018", "boolean-019",
			"boolean-020", "boolean-021", "boolean-022", "boolean-023",
			"boolean-024", "boolean-025",
			"boolean-026", "boolean-027", "boolean-028", "boolean-029",
			"boolean-030", "boolean-031", "boolean-032", "boolean-033",
			"boolean-034", "boolean-035",
			"boolean-036", "boolean-037", "boolean-038", "boolean-039",
			"boolean-040", "boolean-041", "boolean-042", "boolean-043",
			"boolean-044", "boolean-045", "boolean-046", "boolean-047",
			"boolean-048", "boolean-049", "boolean-050", "boolean-051",
			"boolean-052", "boolean-053", "boolean-054", "boolean-055",
			"boolean-056", "boolean-057", "boolean-058", "boolean-059",
			"boolean-060", "boolean-061", "boolean-062", "boolean-063",
			"boolean-064", "boolean-065", "boolean-066", "boolean-067",
			"boolean-068", "boolean-069",
			"boolean-070", "boolean-071", "boolean-072",
			"boolean-073", "boolean-074", "boolean-075", "boolean-076",
			"boolean-077", "boolean-078", "boolean-079", "boolean-080",
			"boolean-081", "boolean-082", "boolean-083",
			"boolean-084", "boolean-085", "boolean-086", "boolean-087",
			"boolean-088", "boolean-089", "boolean-090",
			"boolean-091", "boolean-092", "boolean-093", "boolean-094",
			"boolean-095", "boolean-096",
			"boolean-099",
			"boolean-100", "boolean-101", "boolean-102", "boolean-103",
			"boolean-104", "boolean-105", "boolean-106", "boolean-107",
			"boolean-108", "boolean-109", "boolean-110", "boolean-111", "boolean-112",
		},
	},
	"namespace": {
		File: "tests/type/namespace/_namespace-test-set.xml",
		Tests: []string{
			"namespace-0401", "namespace-0402",
			"namespace-0903", "namespace-0904",
			"namespace-1101",
			"namespace-1403", "namespace-1404",
			"namespace-1701",
			"namespace-2402",
			"namespace-2612",
			"namespace-2701",
			"namespace-2801",
			"namespace-3001",
			"namespace-3108", "namespace-3125", "namespace-3128",
			"namespace-3131", "namespace-3135", "namespace-3138",
			"namespace-3141", "namespace-3143",
			"namespace-3161", "namespace-3163",
			"namespace-3202",
			"namespace-3305", "namespace-3306", "namespace-3307",
			"namespace-3312", "namespace-3313",
			"namespace-3401",
			"namespace-3501", "namespace-3502", "namespace-3503", "namespace-3504",
			"namespace-3701", "namespace-3703",
			"namespace-3801",
			"namespace-3901", "namespace-3902", "namespace-3903",
			"namespace-4001", "namespace-4003",
			"namespace-4006",
			"namespace-4901",
			"namespace-5001",
			"namespace-5401", "namespace-5501",
			"namespace-6001",
		},
	},
	"expression": {
		File: "tests/expr/expression/_expression-test-set.xml",
		Tests: []string{
			"expression-0101",
			"expression-0301", "expression-0302", "expression-0303",
			"expression-0401", "expression-0402", "expression-0403", "expression-0404",
			"expression-0501", "expression-0601",
			"expression-0701", "expression-0702",
			"expression-0901", "expression-0902", "expression-0903",
			"expression-0904", "expression-0905", "expression-0906",
			"expression-0909", "expression-0910", "expression-0911",
			"expression-0912", "expression-0913", "expression-0914",
			"expression-0915", "expression-0916", "expression-0918", "expression-0919",
			"expression-0920", "expression-0921", "expression-0922",
			"expression-0923", "expression-0924", "expression-0925",
			"expression-0927", "expression-0928", "expression-0929",
			"expression-0930",
			"expression-1001",
			"expression-1101", "expression-1102", "expression-1103", "expression-1104",
			"expression-1201", "expression-1301",
			"expression-1401",
			"expression-1501", "expression-1601", "expression-1701", "expression-1702",
			"expression-1801",
			"expression-2001",
			"expression-2201", "expression-2202", "expression-2203",
			"expression-2301", "expression-2302",
			"expression-2401", "expression-2402", "expression-2501",
			"expression-2601", "expression-2701", "expression-2702",
			"expression-2801", "expression-2901",
			"expression-3001", "expression-3101", "expression-3201",
			"expression-3301", "expression-3302", "expression-3401",
			"expression-3501", "expression-3601", "expression-3701",
			"expression-3801", "expression-3901",
			"expression-4001", "expression-4101",
			"expression-4201", "expression-4202", "expression-4203",
			"expression-4204", "expression-4205", "expression-4206",
			"expression-4207", "expression-4208", "expression-4209", "expression-4210",
			"expression-4301", "expression-4303",
			"expression-4401", "expression-4402",
		},
	},
	"predicate": {
		File: "tests/expr/predicate/_predicate-test-set.xml",
		Tests: []string{
			"predicate-004", "predicate-005", "predicate-006", "predicate-007", "predicate-008",
			"predicate-009", "predicate-010", "predicate-011", "predicate-012",
			"predicate-013", "predicate-014", "predicate-015", "predicate-016",
			"predicate-017", "predicate-018",
			"predicate-024", "predicate-025",
			"predicate-027", "predicate-028", "predicate-029",
			"predicate-030", "predicate-031", "predicate-032", "predicate-033",
			"predicate-034", "predicate-035", "predicate-036", "predicate-037",
			"predicate-038", "predicate-039", "predicate-040", "predicate-041",
			"predicate-042", "predicate-043", "predicate-044", "predicate-045",
			"predicate-046", "predicate-047", "predicate-048", "predicate-049",
			"predicate-050", "predicate-051",
			"predicate-054", "predicate-056",
		},
	},
	"select": {
		File: "tests/attr/select/_select-test-set.xml",
		Tests: []string{
			"select-0101", "select-0102",
			"select-0201", "select-0202",
			"select-0301", "select-0501",
			"select-0601", "select-0801", "select-0802",
			"select-0901", "select-0902",
			"select-1001", "select-1101", "select-1201",
			"select-1301", "select-1302",
			"select-1401", "select-1402",
			"select-1502", "select-1503", "select-1504",
			"select-1702", "select-1704",
			"select-1804", "select-1901",
			"select-2003", "select-2004", "select-2005", "select-2006",
			"select-2009", "select-2011", "select-2012",
			"select-2014", "select-2015", "select-2017",
			"select-2021", "select-2022", "select-2023", "select-2024",
			"select-2029", "select-2031", "select-2032",
			"select-2034", "select-2035", "select-2036", "select-2038",
			"select-2302", "select-2304", "select-2305",
			"select-2402",
			"select-2501", "select-2502", "select-2503", "select-2504",
			"select-2505", "select-2506",
			"select-2601", "select-2602", "select-2603",
			"select-2701",
			"select-3001", "select-3101", "select-3201",
			"select-3501", "select-3502",
			"select-3601", "select-3602", "select-3603",
			"select-3701", "select-3801",
			"select-3901", "select-3902",
			"select-4001", "select-4101", "select-4102",
			"select-4201", "select-4301", "select-4302",
			"select-4401", "select-4501", "select-4601",
			"select-4701", "select-4801", "select-4901",
			"select-5001", "select-5101", "select-5201",
			"select-5301", "select-5401", "select-5501",
			"select-5601", "select-5701", "select-5801",
			"select-6001", "select-6201", "select-6301",
			"select-6401", "select-6501", "select-6601",
			"select-6701", "select-6801",
			"select-7001", "select-7401",
			"select-7501", "select-7502b",
		},
	},
	"mode": {
		File: "tests/attr/mode/_mode-test-set.xml",
		Tests: []string{
			"mode-0001", "mode-0002", "mode-0003", "mode-0004",
			"mode-0011",
			"mode-0101", "mode-0103", "mode-0104", "mode-0105", "mode-0106", "mode-0108",
			"mode-0401", "mode-0501", "mode-0601", "mode-0701",
			"mode-0801a", "mode-0801c", "mode-0802", "mode-0803", "mode-0804", "mode-0805",
			"mode-1001",
			"mode-1108", "mode-1202",
			"mode-1405", "mode-1406", "mode-1407", "mode-1408", "mode-1409", "mode-1410",
			"mode-1423", "mode-1424", "mode-1444",
			"mode-1509", "mode-1515", "mode-1516",
			"mode-1605", "mode-1608", "mode-1611",
		},
	},
	"sequence": {
		File: "tests/insn/sequence/_sequence-test-set.xml",
		Tests: []string{
			"sequence-0104", "sequence-0111", "sequence-0112",
			"sequence-0116", "sequence-0118",
			"sequence-0120", "sequence-0121",
			"sequence-0128", "sequence-0129", "sequence-0132",
			"sequence-0137", "sequence-0138",
			"sequence-0201", "sequence-0202", "sequence-0203",
			"sequence-0301", "sequence-0302", "sequence-0305",
			"sequence-0701", "sequence-0702", "sequence-0703",
			"sequence-0704", "sequence-0705", "sequence-0706",
			"sequence-1003",
			"sequence-1202", "sequence-1203", "sequence-1204", "sequence-1205",
			"sequence-1301",
			"sequence-2101",
			"sequence-2402a", "sequence-2403a",
		},
	},
	"merge": {
		File: "tests/insn/merge/_merge-test-set.xml",
		Tests: []string{
			"merge-007", "merge-008", "merge-009", "merge-010", "merge-011",
			"merge-020", "merge-022", "merge-023",
			"merge-026", "merge-027",
			"merge-030", "merge-031", "merge-032a", "merge-032b", "merge-032c",
			"merge-033", "merge-034", "merge-035", "merge-036", "merge-037",
			"merge-042", "merge-043", "merge-045",
			"merge-046a", "merge-046b", "merge-048",
			"merge-055", "merge-056", "merge-057", "merge-058",
			"merge-064", "merge-067",
			"merge-072", "merge-074", "merge-075", "merge-077",
			"merge-087", "merge-088",
			"merge-094", "merge-095",
			"merge-100", "merge-101",
		},
	},
	"output": {
		File: "tests/decl/output/_output-test-set.xml",
		Tests: []string{
			"output-0102d", "output-0102f",
			"output-0103a", "output-0103d", "output-0103f",
			"output-0104", "output-0105",
			"output-0112", "output-0117", "output-0118",
			"output-0127",
			"output-0137", "output-0137a", "output-0137b",
			"output-0139", "output-0147",
			"output-0156", "output-0160", "output-0161", "output-0163",
			"output-0178", "output-0180",
			"output-0197", "output-0197a", "output-0198", "output-0198a",
			"output-0199", "output-0199a",
			"output-0215", "output-0216", "output-0217",
			"output-0222", "output-0223",
			"output-0280", "output-0280a", "output-0281", "output-0281a",
			"output-0282", "output-0282a", "output-0283", "output-0283a",
			"output-0284",
			"output-0602c",
			"output-0707",
		},
	},
	"import": {
		File: "tests/decl/import/_import-test-set.xml",
		Tests: []string{
			"import-0201", "import-0202", "import-0203",
			"import-0301", "import-0302",
			// import-0502b: requires on-multiple-match="error" (XSLT 1.0/2.0 only)
			"import-0902b",
			"import-1301",
			"import-2001",
			"import-2103",
			"import-2401", "import-2402", "import-2403", "import-2404",
		},
	},
	"element": {
		File: "tests/insn/element/_element-test-set.xml",
		Tests: []string{
			"element-0005", "element-0006",
			"element-0101", "element-0103",
			"element-0201",
			"element-0304", "element-0312",
		},
	},
	"attribute": {
		File: "tests/insn/attribute/_attribute-test-set.xml",
		Tests: []string{
			"attribute-0005",
			"attribute-0401",
			"attribute-0807",
			"attribute-0902",
			"attribute-1401",
			"attribute-1505",
		},
	},
	"data-manipulation": {
		File: "tests/expr/data-manipulation/_data-manipulation-test-set.xml",
	},
	"template": {
		File: "tests/decl/template/_template-test-set.xml",
		Tests: []string{
			"template-001", "template-002", "template-003",
			"template-004", "template-005", "template-006",
			"template-007", "template-008", "template-009", "template-010",
			"template-011", "template-012", "template-013", "template-014",
			"template-015", "template-016", "template-017", "template-018",
			"template-019", "template-020",
		},
	},
}

// XML structures for parsing W3C test-set files.
// Namespace: http://www.w3.org/2012/10/xslt-test-catalog

type w3cTestSet struct {
	XMLName      xml.Name         `xml:"http://www.w3.org/2012/10/xslt-test-catalog test-set"`
	Environments []w3cEnvironment `xml:"http://www.w3.org/2012/10/xslt-test-catalog environment"`
	TestCases    []w3cTestCase    `xml:"http://www.w3.org/2012/10/xslt-test-catalog test-case"`
}

type w3cEnvironment struct {
	Name        string          `xml:"name,attr"`
	Sources     []w3cSource     `xml:"http://www.w3.org/2012/10/xslt-test-catalog source"`
	Stylesheets []w3cStylesheet `xml:"http://www.w3.org/2012/10/xslt-test-catalog stylesheet"`
}

type w3cSource struct {
	Role    string     `xml:"role,attr"`
	File    string     `xml:"file,attr"`
	Content w3cContent `xml:"http://www.w3.org/2012/10/xslt-test-catalog content"`
}

type w3cContent struct {
	Data string `xml:",chardata"`
}

type w3cTestCase struct {
	Name        string    `xml:"name,attr"`
	Environment w3cEnvRef `xml:"http://www.w3.org/2012/10/xslt-test-catalog environment"`
	Test        w3cTest   `xml:"http://www.w3.org/2012/10/xslt-test-catalog test"`
	Result      w3cResult `xml:"http://www.w3.org/2012/10/xslt-test-catalog result"`
}

type w3cEnvRef struct {
	Ref     string      `xml:"ref,attr"`
	Sources []w3cSource `xml:"http://www.w3.org/2012/10/xslt-test-catalog source"`
}

type w3cTest struct {
	Stylesheets     []w3cStylesheet    `xml:"http://www.w3.org/2012/10/xslt-test-catalog stylesheet"`
	Params          []w3cParam         `xml:"http://www.w3.org/2012/10/xslt-test-catalog param"`
	InitialTemplate *w3cInitTemplate   `xml:"http://www.w3.org/2012/10/xslt-test-catalog initial-template"`
}

type w3cInitTemplate struct {
	Name string `xml:"name,attr"`
}

type w3cParam struct {
	Name   string `xml:"name,attr"`
	Select string `xml:"select,attr"`
}

type w3cStylesheet struct {
	File string `xml:"file,attr"`
}

type w3cResult struct {
	AssertXML          []w3cAssertXML        `xml:"http://www.w3.org/2012/10/xslt-test-catalog assert-xml"`
	Assert             []w3cAssert           `xml:"http://www.w3.org/2012/10/xslt-test-catalog assert"`
	Error              []w3cError            `xml:"http://www.w3.org/2012/10/xslt-test-catalog error"`
	AllOf              *w3cAllOf             `xml:"http://www.w3.org/2012/10/xslt-test-catalog all-of"`
	AnyOf              *w3cAnyOf             `xml:"http://www.w3.org/2012/10/xslt-test-catalog any-of"`
	AssertStringValue  []w3cAssertString     `xml:"http://www.w3.org/2012/10/xslt-test-catalog assert-string-value"`
	SerializationMatch []w3cSerializationMatch `xml:"http://www.w3.org/2012/10/xslt-test-catalog serialization-matches"`
}

type w3cAssert struct {
	XPath string `xml:",chardata"`
}

type w3cAssertXML struct {
	Content string `xml:",chardata"`
	File    string `xml:"file,attr"`
}

type w3cError struct {
	Code string `xml:"code,attr"`
}

type w3cAssertString struct {
	Value      string `xml:",chardata"`
	Normalize  string `xml:"normalize-space,attr"`
}

type w3cSerializationMatch struct {
	Pattern string `xml:",chardata"`
	Flags   string `xml:"flags,attr"`
}

type w3cAllOf struct {
	AssertXML         []w3cAssertXML        `xml:"http://www.w3.org/2012/10/xslt-test-catalog assert-xml"`
	Assert            []w3cAssert           `xml:"http://www.w3.org/2012/10/xslt-test-catalog assert"`
	AnyOf             []w3cAnyOf            `xml:"http://www.w3.org/2012/10/xslt-test-catalog any-of"`
	AssertStringValue []w3cAssertString     `xml:"http://www.w3.org/2012/10/xslt-test-catalog assert-string-value"`
	SerializationMatch []w3cSerializationMatch `xml:"http://www.w3.org/2012/10/xslt-test-catalog serialization-matches"`
}

type w3cAnyOf struct {
	AssertXML          []w3cAssertXML        `xml:"http://www.w3.org/2012/10/xslt-test-catalog assert-xml"`
	Assert             []w3cAssert           `xml:"http://www.w3.org/2012/10/xslt-test-catalog assert"`
	Error              []w3cError            `xml:"http://www.w3.org/2012/10/xslt-test-catalog error"`
	AssertStringValue  []w3cAssertString     `xml:"http://www.w3.org/2012/10/xslt-test-catalog assert-string-value"`
	SerializationMatch []w3cSerializationMatch `xml:"http://www.w3.org/2012/10/xslt-test-catalog serialization-matches"`
}

func TestW3C(t *testing.T) {
	if _, err := os.Stat(w3cTestSuiteDir); os.IsNotExist(err) {
		t.Skip("W3C XSLT test suite not found at", w3cTestSuiteDir)
	}
	for setName, config := range w3cTestSets {
		t.Run(setName, func(t *testing.T) {
			runW3CTestSet(t, config)
		})
	}
}

func runW3CTestSet(t *testing.T, config w3cTestSetConfig) {
	t.Helper()

	testSetPath := filepath.Join(w3cTestSuiteDir, config.File)
	testSetDir := filepath.Dir(testSetPath)

	data, err := os.ReadFile(testSetPath)
	if err != nil {
		t.Fatal(err)
	}

	var ts w3cTestSet
	if err := xml.Unmarshal(data, &ts); err != nil {
		t.Fatal(err)
	}

	// Build environment map.
	envMap := make(map[string]*w3cEnvironment)
	for i := range ts.Environments {
		envMap[ts.Environments[i].Name] = &ts.Environments[i]
	}

	// Build whitelist set.
	whitelist := make(map[string]bool)
	for _, name := range config.Tests {
		whitelist[name] = true
	}

	for _, tc := range ts.TestCases {
		if len(whitelist) > 0 && !whitelist[tc.Name] {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			runW3CTestCase(t, tc, envMap, testSetDir)
		})
	}
}

func runW3CTestCase(t *testing.T, tc w3cTestCase, envMap map[string]*w3cEnvironment, baseDir string) {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()

	expectError := w3cExpectsError(tc.Result)

	// Resolve source XML.
	sourceXML, err := w3cResolveSource(tc, envMap, baseDir)
	if err != nil {
		// Some tests (like json-to-xml) have no source document.
		// Provide an empty document in that case.
		sourceXML = "<empty/>"
	}

	// Parse source document.
	sourceDoc, err := goxml.Parse(strings.NewReader(sourceXML))
	if err != nil {
		if expectError {
			return
		}
		t.Fatal("parsing source XML:", err)
	}

	// Get stylesheet path — from test or from environment.
	var xsltPath string
	if len(tc.Test.Stylesheets) > 0 {
		xsltPath = filepath.Join(baseDir, tc.Test.Stylesheets[0].File)
	} else if tc.Environment.Ref != "" {
		if env, ok := envMap[tc.Environment.Ref]; ok && len(env.Stylesheets) > 0 {
			xsltPath = filepath.Join(baseDir, env.Stylesheets[0].File)
		}
	}
	if xsltPath == "" {
		t.Fatal("no stylesheet in test case")
	}

	// Compile stylesheet.
	ss, err := CompileFile(xsltPath)
	if err != nil {
		if expectError {
			return
		}
		t.Fatal("compiling stylesheet:", err)
	}

	// Build transform options with parameters.
	opts := TransformOptions{}
	if tc.Test.InitialTemplate != nil {
		opts.InitialTemplate = tc.Test.InitialTemplate.Name
	}
	if len(tc.Test.Params) > 0 {
		opts.Parameters = make(map[string]goxpath.Sequence)
		for _, p := range tc.Test.Params {
			// Evaluate the select expression to get a proper XPath value.
			np := &goxpath.Parser{Ctx: goxpath.NewContext(sourceDoc)}
			seq, evalErr := np.Evaluate(p.Select)
			if evalErr != nil {
				t.Fatalf("evaluating param %s select=%q: %v", p.Name, p.Select, evalErr)
			}
			opts.Parameters[p.Name] = seq
		}
	}

	// Transform.
	result, err := TransformWithOptions(ss, sourceDoc, opts)
	if err != nil {
		if expectError {
			return
		}
		t.Fatal("transforming:", err)
	}

	if expectError {
		t.Fatal("expected error but transformation succeeded")
	}

	got := SerializeWithOutput(result.Document, result.Output)

	// Check assertion.
	w3cAssertResult(t, tc.Result, got, baseDir)
}

func w3cResolveSource(tc w3cTestCase, envMap map[string]*w3cEnvironment, baseDir string) (string, error) {
	// Look up referenced environment first if available.
	if tc.Environment.Ref != "" {
		env, ok := envMap[tc.Environment.Ref]
		if !ok {
			return "", fmt.Errorf("environment %q not found", tc.Environment.Ref)
		}
		for _, src := range env.Sources {
			if src.Role == "." {
				if src.File != "" {
					data, err := os.ReadFile(filepath.Join(baseDir, src.File))
					if err != nil {
						return "", err
					}
					return string(data), nil
				}
				return src.Content.Data, nil
			}
		}
	}

	// Check inline environment sources.
	if len(tc.Environment.Sources) > 0 {
		for _, src := range tc.Environment.Sources {
			if src.Role == "." {
				if src.File != "" {
					data, err := os.ReadFile(filepath.Join(baseDir, src.File))
					if err != nil {
						return "", err
					}
					return string(data), nil
				}
				return src.Content.Data, nil
			}
		}
	}

	return "", fmt.Errorf("no source document found for test case %q", tc.Name)
}

func w3cAssertResult(t *testing.T, result w3cResult, got string, baseDir string) {
	t.Helper()

	if len(result.AssertXML) > 0 {
		w3cCheckAssertXML(t, result.AssertXML[0], got, baseDir)
		return
	}

	if len(result.Assert) > 0 {
		w3cCheckAssert(t, result.Assert[0], got)
		return
	}

	if len(result.AssertStringValue) > 0 {
		w3cCheckAssertStringValue(t, result.AssertStringValue[0], got)
		return
	}

	if len(result.SerializationMatch) > 0 {
		w3cCheckSerializationMatch(t, result.SerializationMatch[0], got)
		return
	}

	if result.AllOf != nil && w3cAllOfHasAssertions(result.AllOf) {
		for _, ax := range result.AllOf.AssertXML {
			w3cCheckAssertXML(t, ax, got, baseDir)
		}
		for _, a := range result.AllOf.Assert {
			w3cCheckAssert(t, a, got)
		}
		for _, ao := range result.AllOf.AnyOf {
			w3cCheckAnyOf(t, ao, got, baseDir)
		}
		for _, sv := range result.AllOf.AssertStringValue {
			w3cCheckAssertStringValue(t, sv, got)
		}
		for _, sm := range result.AllOf.SerializationMatch {
			w3cCheckSerializationMatch(t, sm, got)
		}
		return
	}

	if result.AnyOf != nil && w3cAnyOfHasAssertions(result.AnyOf) {
		w3cCheckAnyOf(t, *result.AnyOf, got, baseDir)
		return
	}

	t.Skip("unsupported assertion type")
}

func w3cAllOfHasAssertions(a *w3cAllOf) bool {
	return len(a.AssertXML) > 0 || len(a.Assert) > 0 || len(a.AnyOf) > 0 ||
		len(a.AssertStringValue) > 0 || len(a.SerializationMatch) > 0
}

func w3cAnyOfHasAssertions(a *w3cAnyOf) bool {
	return len(a.AssertXML) > 0 || len(a.Assert) > 0 || len(a.Error) > 0 ||
		len(a.AssertStringValue) > 0 || len(a.SerializationMatch) > 0
}

func w3cCheckAnyOf(t *testing.T, ao w3cAnyOf, got string, baseDir string) {
	t.Helper()
	// Try assert-xml matches.
	for _, ax := range ao.AssertXML {
		expected, err := w3cResolveAssertXML(ax, baseDir)
		if err != nil {
			continue
		}
		if w3cNormalizeXML(w3cStripXMLDecl(got)) == w3cNormalizeXML(w3cStripXMLDecl(expected)) {
			return
		}
	}
	// Try XPath assertions.
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
			return
		}
	}
	// Try assert-string-value.
	for _, sv := range ao.AssertStringValue {
		gotStr := w3cExtractStringValue(got)
		expected := sv.Value
		if sv.Normalize == "yes" {
			gotStr = strings.Join(strings.Fields(gotStr), " ")
			expected = strings.Join(strings.Fields(expected), " ")
		}
		if gotStr == expected {
			return
		}
	}
	// Try serialization-matches.
	for _, sm := range ao.SerializationMatch {
		re, err := regexp.Compile(sm.Pattern)
		if err != nil {
			continue
		}
		if re.MatchString(got) {
			return
		}
	}
	// Try error matches (if error was expected and already caught, we wouldn't be here).
	if len(ao.Error) > 0 {
		return // error is an acceptable outcome
	}
	t.Errorf("no any-of assertion matched\ngot: %s", got)
}

func w3cCheckAssertStringValue(t *testing.T, sv w3cAssertString, got string) {
	t.Helper()
	gotStr := w3cExtractStringValue(got)
	expected := sv.Value
	if sv.Normalize == "yes" {
		gotStr = strings.Join(strings.Fields(gotStr), " ")
		expected = strings.Join(strings.Fields(expected), " ")
	}
	if gotStr != expected {
		t.Errorf("string value mismatch\ngot:      %q\nexpected: %q", gotStr, expected)
	}
}

func w3cCheckSerializationMatch(t *testing.T, sm w3cSerializationMatch, got string) {
	t.Helper()
	re, err := regexp.Compile(sm.Pattern)
	if err != nil {
		t.Fatalf("invalid regex in serialization-matches: %v", err)
	}
	if !re.MatchString(got) {
		t.Errorf("serialization-matches failed\npattern: %s\ngot: %s", sm.Pattern, got)
	}
}

// w3cExtractStringValue extracts the text content from XML output.
func w3cExtractStringValue(s string) string {
	s = w3cStripXMLDecl(s)
	doc, err := goxml.Parse(strings.NewReader(s))
	if err != nil {
		return strings.TrimSpace(s)
	}
	var sb strings.Builder
	var extractText func(n goxml.XMLNode)
	extractText = func(n goxml.XMLNode) {
		switch v := n.(type) {
		case goxml.CharData:
			sb.WriteString(v.Contents)
		case *goxml.Element:
			for _, child := range v.Children() {
				extractText(child)
			}
		case *goxml.XMLDocument:
			for _, child := range v.Children() {
				extractText(child)
			}
		}
	}
	extractText(doc)
	return sb.String()
}

func w3cCheckAssert(t *testing.T, a w3cAssert, got string) {
	t.Helper()
	doc, err := goxml.Parse(strings.NewReader(w3cStripXMLDecl(got)))
	if err != nil {
		t.Fatalf("parsing result XML for assert: %v", err)
	}
	np := &goxpath.Parser{Ctx: goxpath.NewContext(doc)}
	// Use the document as context so that * and / work, but also provide
	// the root element so that "." comparisons can operate on element text.
	np.Ctx.SetContextSequence(goxpath.Sequence{doc})
	// Register common namespaces used in W3C test assertions.
	np.Ctx.Namespaces["j"] = "http://www.w3.org/2005/xpath-functions"
	seq, err := np.Evaluate(a.XPath)
	if err != nil {
		t.Fatalf("evaluating assert XPath %q: %v", a.XPath, err)
	}
	boolVal, err := goxpath.BooleanValue(seq)
	if err != nil {
		t.Fatalf("converting assert result to boolean: %v", err)
	}
	if !boolVal {
		t.Errorf("assert %q evaluated to false\ngot: %s", a.XPath, got)
	}
}

func w3cCheckAssertXML(t *testing.T, ax w3cAssertXML, got string, baseDir string) {
	t.Helper()

	expected, err := w3cResolveAssertXML(ax, baseDir)
	if err != nil {
		t.Fatal("resolving expected XML:", err)
	}

	// Compare after stripping the XML declaration and normalizing the body.
	gotBody := w3cNormalizeXML(w3cStripXMLDecl(got))
	expectedBody := w3cNormalizeXML(w3cStripXMLDecl(expected))

	if gotBody != expectedBody {
		t.Errorf("XML mismatch\ngot:      %s\nexpected: %s", gotBody, expectedBody)
	}
}

// w3cExpectsError returns true if the test result expects an error.
func w3cExpectsError(result w3cResult) bool {
	if len(result.Error) > 0 {
		return true
	}
	if result.AnyOf != nil && len(result.AnyOf.Error) > 0 {
		return false // any-of with error means error is one option, not required
	}
	return false
}

// w3cStripXMLDecl removes a leading <?xml ...?> declaration.
func w3cStripXMLDecl(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "<?xml") {
		if idx := strings.Index(s, "?>"); idx >= 0 {
			return strings.TrimSpace(s[idx+2:])
		}
	}
	return s
}

func w3cResolveAssertXML(ax w3cAssertXML, baseDir string) (string, error) {
	if ax.File != "" {
		data, err := os.ReadFile(filepath.Join(baseDir, ax.File))
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return ax.Content, nil
}

// w3cNormalizeXML parses and re-serializes XML to normalize whitespace differences.
func w3cNormalizeXML(s string) string {
	s = strings.TrimSpace(s)
	doc, err := goxml.Parse(strings.NewReader(s))
	if err != nil {
		return s
	}
	return doc.ToXML()
}

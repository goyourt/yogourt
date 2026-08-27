package providers

import (
	"bytes"
	"log"
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func mappingOf(t *testing.T, content string) *yaml.Node {
	t.Helper()

	var document yaml.Node
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatalf("unexpected YAML error: %v", err)
	}

	root := mappingRoot(&document)
	if root == nil {
		t.Fatalf("expected a mapping at the root of %q", content)
	}
	return root
}

func TestUnknownKeysReportsUndeclaredMainConfigKeys(t *testing.T) {
	root := mappingOf(t, `
app_name: "demo"
paths:
  api_folder: "./api/"
  route_folder: "./api/"
database:
  dbname: "mydb"
  db: "mydb"
`)

	got := unknownKeys(root, reflect.TypeOf(&MainConfig{}))

	want := []string{"paths.api_folder", "database.dbname"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unknownKeys() = %#v, want %#v", got, want)
	}
}

func TestUnknownKeysAcceptsEveryDeclaredKeyOfTheExampleConfig(t *testing.T) {
	root := mappingOf(t, `
app_name: "demo"
version: "1.0.0"
mode: "development"
env_files: "./configs/yogourt.env"
server:
  port: 8080
  host: "0.0.0.0"
  cors: true
  base_path: "/api"
database:
  user: "admin"
  password: "password"
  host: "localhost"
  port: 5432
  db: "mydb"
cache:
  host: "localhost"
  port: "6379"
  password: ""
  db: 0
paths:
  route_folder: "./api/"
security:
  secret_key: "dev"
  hash_cost: 12
  token_expires: 1440
  password_minimum_length: 8
  password_special_char_required: false
  password_number_required: false
  password_upper_case_required: false
  password_lower_case_required: false
cors:
  allow_all_origins: false
  allowed_origins:
    - http://localhost:3000
  allowed_methods:
    - GET
  allowed_headers:
    - Authorization
  allow_credentials: true
  max_age: 12h
`)

	if got := unknownKeys(root, reflect.TypeOf(&MainConfig{})); len(got) != 0 {
		t.Fatalf("expected no unknown key, got %#v", got)
	}
}

func TestUnknownKeysTreatsMapKeysAsData(t *testing.T) {
	root := mappingOf(t, `
file_folder: "./public/files/"
files:
  scripts:
    max_file_size: 2621440
  images:
    typo_folder: "/var/tmp/"
`)

	got := unknownKeys(root, reflect.TypeOf(&FileConfig{}))

	want := []string{"files.images.typo_folder"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unknownKeys() = %#v, want %#v", got, want)
	}
}

func TestUnknownKeysDoesNotWalkIntoCustomUnmarshalers(t *testing.T) {
	// env_files decodes a scalar as well as a list: whatever it holds is its
	// own business, not a set of field names.
	root := mappingOf(t, `
env_files:
  - ./configs/yogourt.env
  - ./configs/local.env
`)

	if got := unknownKeys(root, reflect.TypeOf(&MainConfig{})); len(got) != 0 {
		t.Fatalf("expected no unknown key, got %#v", got)
	}
}

func TestRenamedAndRemovedKeysAreNotDeclaredByTheStruct(t *testing.T) {
	// Both lists only describe keys the struct does not have: a key that came
	// back as a field would be reported as dead while being read.
	for _, keys := range []map[string]string{renamedMainConfigKeys, removedMainConfigKeys} {
		for key := range keys {
			root := mappingOf(t, nestedKeyYAML(key))
			if got := unknownKeys(root, reflect.TypeOf(&MainConfig{})); len(got) != 1 || got[0] != key {
				t.Errorf("%q should be an unknown key of MainConfig, got %#v", key, got)
			}
		}
	}
}

// nestedKeyYAML builds the smallest config file holding a dotted key, so the
// key can be checked against MainConfig.
func nestedKeyYAML(key string) string {
	segments := strings.Split(key, ".")

	content := ""
	for depth, segment := range segments {
		indent := ""
		for i := 0; i < depth; i++ {
			indent += "  "
		}
		content += indent + segment + ":"
		if depth == len(segments)-1 {
			content += " null"
		}
		content += "\n"
	}
	return content
}

func TestWarnAboutDeadConfigKeysNamesRenamedRemovedAndUnknownKeys(t *testing.T) {
	content := []byte(`
app_name: "demo"
server:
  port: 8080
  cors: true
  base_path: "/v1"
database:
  type: "postgres"
  db: "mydb"
paths:
  api_folder: "./api/"
  model_folder: "./models/"
tpyo: true
`)

	logged := captureLog(t, func() {
		warnAboutDeadConfigKeys("configs/yogourt.yaml", content, &MainConfig{})
	})

	for _, want := range []string{
		`"paths.api_folder" was renamed to "paths.route_folder"`,
		`"paths.model_folder" is not a runtime setting (only the v0.5 CLI reads it) — delete it`,
		`unknown key "tpyo"`,
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("missing warning %s in:\n%s", want, logged)
		}
	}

	// Every remaining key of the file is read by the framework: nothing else
	// deserves a line.
	for _, unwanted := range []string{"server.cors", "server.base_path", "server.port", "database.type", "database.db", "app_name"} {
		if strings.Contains(logged, unwanted) {
			t.Errorf("live key %q should not be warned about, got:\n%s", unwanted, logged)
		}
	}
}

func TestWarnAboutDeadConfigKeysKeepsMainConfigRulesOutOfOtherFiles(t *testing.T) {
	// files.yaml has no notion of a renamed or removed key: only the unknown
	// ones are worth a line.
	content := []byte("file_folder: \"./public/files/\"\npaths:\n  model_folder: \"./models/\"\n")

	logged := captureLog(t, func() {
		warnAboutDeadConfigKeys("configs/files.yaml", content, &FileConfig{})
	})

	if !strings.Contains(logged, `unknown key "paths"`) {
		t.Errorf("expected the unknown section to be reported, got:\n%s", logged)
	}
	if strings.Contains(logged, "not a runtime setting") {
		t.Errorf("main config rules leaked into files.yaml:\n%s", logged)
	}
}

func captureLog(t *testing.T, run func()) string {
	t.Helper()

	buffer := &bytes.Buffer{}
	flags := log.Flags()
	log.SetOutput(buffer)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flags)
	})

	run()

	return buffer.String()
}

package server

import (
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSanitizeFieldKeyMakesAParameterName(t *testing.T) {
	cases := map[string]string{
		"Search":     "search",
		"e-mail":     "e_mail",
		"ポスト本文":      "",
		"__x__y__":   "x_y",
		"2nd Field":  "f_2nd_field",
		"My  Field!": "my_field",
	}
	for in, want := range cases {
		if got := sanitizeFieldKey(in); got != want {
			t.Errorf("sanitizeFieldKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnrichElementsKeysAreParameterNames(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	els := enrichElements([]WebWantElement{
		{Role: "textbox", Name: "ポスト本文", Selector: `[aria-label="ポスト本文"]`},
		{Role: "button", Name: "ポストする", Selector: `[data-testid="tweetButtonInline"]`},
		{Role: "button", Name: "送信", Selector: "form > button"},
	})
	seen := map[string]bool{}
	for _, el := range els {
		if !valid.MatchString(el.FieldKey) {
			t.Errorf("%q got key %q, not a parameter name", el.Name, el.FieldKey)
		}
		if seen[el.FieldKey] {
			t.Errorf("key %q given twice", el.FieldKey)
		}
		seen[el.FieldKey] = true
	}
}

// Every saved object becomes a parameter of the type: titled with its own name,
// a field as a string and a button as a bool that is on unless turned off, each
// carrying its picture when there is one.
func TestBuildWebWantYAMLObjectsAreParameters(t *testing.T) {
	els := enrichElements([]WebWantElement{
		{Role: "textbox", Name: `ポスト"本文"`, Selector: "#text"},
		{Role: "button", Name: "ポストする", Selector: "#post"},
	})
	images := map[string]string{els[0].FieldKey: "/api/v1/screenshots/web-t-obj-" + els[0].FieldKey + ".jpg"}
	out := buildWebWantYAML("t_web", "T", "https://t.example/", "t.example", "", "", false, els, images)

	var doc struct {
		WantType struct {
			Parameters []struct {
				Name            string `yaml:"name"`
				Title           string `yaml:"title"`
				Type            string `yaml:"type"`
				Default         any    `yaml:"default"`
				BackgroundImage string `yaml:"backgroundImage"`
			} `yaml:"parameters"`
		} `yaml:"wantType"`
	}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("generated YAML does not parse: %v\n%s", err, out)
	}
	ps := doc.WantType.Parameters
	if len(ps) != 3 {
		t.Fatalf("want target_url + 2 object parameters, got %d", len(ps))
	}
	field, button := ps[1], ps[2]
	if field.Name != els[0].FieldKey || field.Title != `ポスト"本文"` || field.Type != "string" || field.Default != "" {
		t.Errorf("field parameter = %+v", field)
	}
	if field.BackgroundImage != images[els[0].FieldKey] {
		t.Errorf("field backgroundImage = %q", field.BackgroundImage)
	}
	if button.Name != els[1].FieldKey || button.Title != "ポストする" || button.Type != "bool" || button.Default != true {
		t.Errorf("button parameter = %+v", button)
	}
	if button.BackgroundImage != "" {
		t.Errorf("button without a picture got backgroundImage %q", button.BackgroundImage)
	}
}

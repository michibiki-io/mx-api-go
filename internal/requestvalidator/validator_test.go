package requestvalidator

import (
	"testing"

	"github.com/michibiki-io/mx-api-go/internal/config"
)

func TestValidateDefaultRules(t *testing.T) {
	cfg := config.Default()
	engine, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	errors := engine.Validate(map[string]string{
		"name":    "",
		"email":   "invalid",
		"message": "hello",
	})

	if errors["name"].Code != "validation_required" {
		t.Fatalf("name error = %#v", errors["name"])
	}
	if errors["email"].Code != "validation_is_email" {
		t.Fatalf("email error = %#v", errors["email"])
	}
	if _, ok := errors["tel"]; ok {
		t.Fatalf("optional tel should not fail when empty: %#v", errors["tel"])
	}
}

func TestValidateConfiguredRule(t *testing.T) {
	cfg := config.Default()
	cfg.Form.Fields = append(cfg.Form.Fields, config.FieldConfig{
		Name:  "plan",
		Rules: []string{"plan"},
	})
	cfg.Validation["plan"] = config.ValidationRule{
		Tag:     "oneof=basic business",
		Code:    "validation_oneof",
		Message: "must be a configured plan",
	}

	engine, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	errors := engine.Validate(map[string]string{
		"name":    "Jane",
		"email":   "jane@example.com",
		"message": "hello",
		"plan":    "enterprise",
	})

	if errors["plan"].Code != "validation_oneof" {
		t.Fatalf("plan error = %#v", errors["plan"])
	}
}

func TestValidateJPPhone(t *testing.T) {
	cfg := config.Default()
	engine, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		tel     string
		wantErr bool
	}{
		{name: "mobile hyphenated", tel: "080-9990-1111"},
		{name: "mobile digits", tel: "08099901111"},
		{name: "tokyo fixed line", tel: "03-1234-5678"},
		{name: "toll free", tel: "0120-123-456"},
		{name: "too long mobile", tel: "08099901111111111", wantErr: true},
		{name: "not domestic", tel: "+81-80-9990-1111", wantErr: true},
		{name: "letters", tel: "080-ABCD-1111", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := engine.Validate(map[string]string{
				"name":    "Jane",
				"email":   "jane@example.com",
				"message": "hello",
				"tel":     tt.tel,
			})
			_, gotErr := errors["tel"]
			if gotErr != tt.wantErr {
				t.Fatalf("tel error = %v, want %v; errors = %#v", gotErr, tt.wantErr, errors)
			}
		})
	}
}

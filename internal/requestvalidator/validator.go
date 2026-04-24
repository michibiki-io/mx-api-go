package requestvalidator

import (
	"fmt"
	"regexp"
	"strings"

	playground "github.com/go-playground/validator/v10"
	"github.com/michibiki-io/mx-api-go/internal/config"
)

var jpPhoneAllowedChars = regexp.MustCompile(`^[0-9()\-\s]+$`)

type FieldError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Engine struct {
	cfg       *config.Config
	validate  *playground.Validate
	patterns  map[string]*regexp.Regexp
	ruleNames map[string]struct{}
}

func New(cfg *config.Config) (*Engine, error) {
	v := playground.New(playground.WithRequiredStructEnabled())
	if err := v.RegisterValidation("jp_phone", validateJPPhone); err != nil {
		return nil, err
	}

	engine := &Engine{
		cfg:       cfg,
		validate:  v,
		patterns:  map[string]*regexp.Regexp{},
		ruleNames: map[string]struct{}{},
	}
	for name, rule := range cfg.Validation {
		engine.ruleNames[name] = struct{}{}
		if rule.Pattern != "" {
			compiled, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return nil, fmt.Errorf("compile validation pattern %q: %w", name, err)
			}
			engine.patterns[name] = compiled
		}
	}
	return engine, nil
}

func (e *Engine) Validate(values map[string]string) map[string]FieldError {
	errors := map[string]FieldError{}
	for _, field := range e.cfg.Form.Fields {
		value := strings.TrimSpace(values[field.Name])
		for _, ruleName := range field.Rules {
			rule, ok := e.cfg.Validation[ruleName]
			if !ok {
				continue
			}
			if value == "" && ruleName != "required" {
				continue
			}
			if pattern, ok := e.patterns[ruleName]; ok {
				if !pattern.MatchString(value) {
					errors[field.Name] = FieldError{Code: fallback(rule.Code, "validation_"+ruleName), Message: fallback(rule.Message, "is invalid")}
					break
				}
				continue
			}
			if rule.Tag == "" {
				continue
			}
			if err := e.validate.Var(value, rule.Tag); err != nil {
				errors[field.Name] = FieldError{Code: fallback(rule.Code, "validation_"+ruleName), Message: fallback(rule.Message, err.Error())}
				break
			}
		}
	}
	return errors
}

func validateJPPhone(fl playground.FieldLevel) bool {
	value := strings.TrimSpace(fl.Field().String())
	if value == "" {
		return true
	}
	if !jpPhoneAllowedChars.MatchString(value) {
		return false
	}

	normalized := strings.NewReplacer("-", "", "(", "", ")", "", " ", "", "\t", "", "\n", "", "\r", "").Replace(value)
	if !strings.HasPrefix(normalized, "0") {
		return false
	}

	switch {
	case strings.HasPrefix(normalized, "0120"), strings.HasPrefix(normalized, "0570"):
		return len(normalized) == 10
	case strings.HasPrefix(normalized, "0800"):
		return len(normalized) == 11
	case strings.HasPrefix(normalized, "050"),
		strings.HasPrefix(normalized, "070"),
		strings.HasPrefix(normalized, "080"),
		strings.HasPrefix(normalized, "090"):
		return len(normalized) == 11
	default:
		return len(normalized) == 10
	}
}

func fallback(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

package service

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCalculateMarkedUpPrice(t *testing.T) {
	tests := []struct {
		name     string
		upstream string
		markup   string
		rounding string
		expected string
	}{
		{"zero markup returns original", "10.00", "0", "none", "10.00"},
		{"50% markup", "10.00", "50", "none", "15.00"},
		{"100% markup doubles price", "10.00", "100", "none", "20.00"},
		{"200% markup triples price", "10.00", "200", "none", "30.00"},
		{"small price with markup", "0.01", "100", "none", "0.02"},
		{"fractional result rounds to 2 decimals", "10.00", "33", "none", "13.30"},
		{"negative markup floors at zero", "5.00", "-200", "none", "0"},
		{"ceil_int rounds up", "10.00", "23.4", "ceil_int", "13.00"},
		{"ceil_int exact integer stays", "10.00", "100", "ceil_int", "20"},
		{"ceil_tenth rounds up to 0.1", "10.00", "23.4", "ceil_tenth", "12.40"},
		{"ceil_tenth exact tenth stays", "10.00", "50", "ceil_tenth", "15.00"},
		{"zero price stays zero", "0.00", "100", "none", "0.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := decimal.RequireFromString(tt.upstream)
			markup := decimal.RequireFromString(tt.markup)
			expected := decimal.RequireFromString(tt.expected)

			result := CalculateMarkedUpPrice(upstream, markup, tt.rounding)
			if !result.Equal(expected) {
				t.Errorf("CalculateMarkedUpPrice(%s, %s%%, %s) = %s, want %s",
					tt.upstream, tt.markup, tt.rounding, result.String(), tt.expected)
			}
		})
	}
}

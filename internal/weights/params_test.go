package weights

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestDefaultParams_AreValid(t *testing.T) {
	if err := DefaultParams().Validate(); err != nil {
		t.Fatalf("expected default parameters to be valid, got %v", err)
	}
}

func TestParams_ValidateRejectsOutOfRange(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Params)
		wantErr error
	}{
		{name: "negative alpha", mutate: func(p *Params) { p.Alpha = -1 }, wantErr: ErrInvalidAlpha},
		{name: "nan alpha", mutate: func(p *Params) { p.Alpha = math.NaN() }, wantErr: ErrInvalidAlpha},
		{name: "zero beta", mutate: func(p *Params) { p.Beta = 0 }, wantErr: ErrInvalidBeta},
		{name: "zero gamma", mutate: func(p *Params) { p.Gamma = 0 }, wantErr: ErrInvalidGamma},
		{name: "gamma above one", mutate: func(p *Params) { p.Gamma = 1.5 }, wantErr: ErrInvalidGamma},
		{name: "negative lambda", mutate: func(p *Params) { p.Lambda = -0.1 }, wantErr: ErrInvalidLambda},
		{name: "lambda of one", mutate: func(p *Params) { p.Lambda = 1 }, wantErr: ErrInvalidLambda},
		{name: "zero delta", mutate: func(p *Params) { p.Delta = 0 }, wantErr: ErrInvalidDelta},
		{name: "negative hold", mutate: func(p *Params) { p.Hold = -time.Second }, wantErr: ErrInvalidHold},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultParams()
			tt.mutate(&p)

			err := p.Validate()
			if err == nil {
				t.Fatalf("expected an error for %s, got nil", tt.name)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestParams_ValidateReportsEveryProblem(t *testing.T) {
	p := Params{} // zero value: beta, gamma and delta are all invalid

	err := p.Validate()
	if err == nil {
		t.Fatal("expected errors for the zero value, got nil")
	}
	for _, want := range []error{ErrInvalidBeta, ErrInvalidGamma, ErrInvalidDelta} {
		if !errors.Is(err, want) {
			t.Errorf("expected the joined error to match %v, got %v", want, err)
		}
	}
}

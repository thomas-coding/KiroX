package core

import (
	"testing"

	"reg_go/internal/email"
)

type recordingEmailService struct {
	timeout  int
	interval int
}

func (s *recordingEmailService) Create() string {
	return "leased@gmail.com"
}

func (s *recordingEmailService) GetAddress() string {
	return "leased@gmail.com"
}

func (s *recordingEmailService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	s.timeout = timeoutSec
	s.interval = intervalSec
	return "123456", nil
}

var _ email.TempEmailService = (*recordingEmailService)(nil)

func TestStep10GetOTPSmsbUsesShortTimeout(t *testing.T) {
	service := &recordingEmailService{}
	registrar := &Registrar{
		Cfg:      &Config{UseSmsbGmail: true},
		EmailSvc: service,
	}

	code, err := registrar.Step10GetOTP()
	if err != nil {
		t.Fatalf("Step10GetOTP() error = %v", err)
	}
	if code != "123456" {
		t.Fatalf("Step10GetOTP() code = %q", code)
	}
	if service.timeout != 30 || service.interval != 3 {
		t.Fatalf("WaitForCode timeout/interval = %d/%d, want 30/3", service.timeout, service.interval)
	}
}

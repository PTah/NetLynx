package netutil

import "testing"

func TestValidateOutboundURL_Webhook(t *testing.T) {
	p := WebhookPolicy()
	if err := ValidateOutboundURL("https://example.com/hook", p); err != nil {
		t.Fatalf("public https: %v", err)
	}
	if err := ValidateOutboundURL("http://example.com/hook", p); err == nil {
		t.Fatal("http must fail for webhook")
	}
	if err := ValidateOutboundURL("https://127.0.0.1/hook", p); err == nil {
		t.Fatal("loopback must fail")
	}
	if err := ValidateOutboundURL("https://192.168.1.1/hook", p); err == nil {
		t.Fatal("private must fail for webhook")
	}
	if err := ValidateOutboundURL("https://169.254.169.254/latest", p); err == nil {
		t.Fatal("link-local metadata must fail")
	}
}

func TestValidateOutboundURL_UISP(t *testing.T) {
	p := UISPPolicy()
	if err := ValidateOutboundURL("http://192.168.1.50:8443", p); err != nil {
		t.Fatalf("LAN UISP http: %v", err)
	}
	if err := ValidateOutboundURL("https://8.8.8.8", p); err != nil {
		t.Fatalf("public https IP: %v", err)
	}
	if err := ValidateOutboundURL("http://127.0.0.1:8080", p); err == nil {
		t.Fatal("loopback must fail for UISP")
	}
	if err := ValidateOutboundURL("http://169.254.169.254/", p); err == nil {
		t.Fatal("metadata must fail")
	}
}

func TestValidateDeviceHost(t *testing.T) {
	if err := ValidateDeviceHost("192.168.1.10"); err != nil {
		t.Fatalf("LAN: %v", err)
	}
	if err := ValidateDeviceHost("127.0.0.1"); err == nil {
		t.Fatal("loopback IP")
	}
	if err := ValidateDeviceHost("localhost"); err == nil {
		t.Fatal("localhost")
	}
	if err := ValidateDeviceHost("::1"); err == nil {
		t.Fatal("IPv6 loopback")
	}
	if err := ValidateDeviceHost("169.254.169.254"); err == nil {
		t.Fatal("metadata")
	}
	if err := ValidateDeviceHost("core-sw"); err != nil {
		t.Fatalf("hostname: %v", err)
	}
}

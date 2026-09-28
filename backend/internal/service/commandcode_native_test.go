package service

import "testing"

func TestCommandCodeNativeAccountContracts(t *testing.T) {
	first := &Account{Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "native-key-one"}}
	second := &Account{Platform: PlatformCommandCode, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "native-key-two"}}
	t.Run("compatible-routing", func(t *testing.T) {
		if !first.IsOpenAICompatible() {
			t.Fatal("native CommandCode must participate in OpenAI-compatible routing")
		}
	})
	t.Run("independent-credentials", func(t *testing.T) {
		if got := first.GetOpenAIProtocolAPIKey(); got != "native-key-one" {
			t.Fatalf("first account key = %q", got)
		}
		if got := second.GetOpenAIProtocolAPIKey(); got != "native-key-two" {
			t.Fatalf("second account key = %q", got)
		}
		if got := first.GetOpenAIProtocolAPIKey(); got != "native-key-one" {
			t.Fatalf("first account key changed = %q", got)
		}
	})
	t.Run("native-default-url", func(t *testing.T) {
		if got := first.GetOpenAIBaseURL(); got != "https://api.commandcode.ai" {
			t.Fatalf("native base URL = %q", got)
		}
	})
}

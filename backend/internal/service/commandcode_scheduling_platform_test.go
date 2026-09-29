//go:build unit

package service

import "testing"

func TestCommandCodeSchedulingPlatformPreserved(t *testing.T) {
	for _, platform := range []string{PlatformCommandCode, PlatformOpenAI, PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo} {
		t.Run(platform, func(t *testing.T) {
			if got := NormalizeOpenAICompatiblePlatform(platform); got != platform {
				t.Fatalf("NormalizeOpenAICompatiblePlatform(%q) = %q; want %q", platform, got, platform)
			}
		})
	}
	if got := NormalizeOpenAICompatiblePlatform("unknown"); got != PlatformOpenAI {
		t.Fatalf("unknown platform = %q; want openai", got)
	}
}

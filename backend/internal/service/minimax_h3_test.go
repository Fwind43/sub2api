package service

import "testing"

func TestH3RejectsNonVideoPayloads(t *testing.T) {
	for _, body := range [][]byte{nil, []byte(`{"status":"completed","url":"https://example.org/video.mp4"}`), []byte("<html>not a video</html>"), make([]byte, 2048)} {
		if verifyH3Video(body) {
			t.Fatal("accepted a non-video payload")
		}
	}
}
func TestH3RequestContract(t *testing.T) {
	good := []byte(`{"model":"MiniMax-H3","prompt":"waves","duration":6}`)
	s, err := ParseH3Request(good)
	if err != nil || s.Seconds != 6 || s.Resolution != "720p" {
		t.Fatalf("invalid defaults: %+v %v", s, err)
	}
	for _, body := range []string{
		`{"model":"grok-video","prompt":"waves"}`,
		`{"model":"MiniMax-H3","prompt":"waves","duration":16}`,
		`{"model":"MiniMax-H3","prompt":"waves","duration":6,"seconds":8}`,
		`{"model":"MiniMax-H3","prompt":"waves","n":2}`,
	} {
		if _, err := ParseH3Request([]byte(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
func TestH3UpstreamIDBoundary(t *testing.T) {
	for _, id := range []string{"", "../task", "a?token=x", "a/b", "a\n"} {
		if validH3UpstreamID(id) {
			t.Fatalf("accepted %q", id)
		}
	}
	if !validH3UpstreamID("task-123") {
		t.Fatal("rejected safe ID")
	}
}

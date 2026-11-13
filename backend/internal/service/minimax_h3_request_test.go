package service

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// These tiny fixtures exercise the JSON/base64 contract, not media decoding.
const h3TestImage = "data:image/png;base64,AQID"

func h3TestReference(kind, mime string) map[string]any {
	return map[string]any{"type": kind, "role": "reference", "data": "data:" + mime + ";base64,AQID"}
}

func h3TestKeyframe(frame int) map[string]any {
	return map[string]any{"type": "image", "role": "keyframe", "frame_index": frame, "data": h3TestImage}
}

func h3TestBody(t *testing.T, task string, conditions []map[string]any) []byte {
	t.Helper()
	payload := map[string]any{"model": H3Model, "prompt": "Use <Picture 1>, <Video 1> and <Audio 1>",
		"seconds": 5, "resolution": "768p", "n": 1, "conditions": conditions,
		"quality": "final", "aspect_ratio": "16:9", "seed": 1101, "upscale": "none",
		"extension": map[string]any{"preserve": true}}
	if task != "" {
		payload["task"] = task
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestH3ReferenceModesAndPreservation(t *testing.T) {
	image := h3TestReference("image", "image/jpeg")
	image["extension"] = map[string]any{"preserve": true}
	video := h3TestReference("video", "video/mp4")
	audio := h3TestReference("audio", "audio/wav")
	av := h3TestReference("video_audio", "video/mp4")
	for _, tc := range []struct {
		name, task string
		conditions []map[string]any
	}{
		{"text", "t2va", nil},
		{"first", "fl2va", []map[string]any{h3TestKeyframe(0)}},
		{"last", "fl2va", []map[string]any{h3TestKeyframe(-1)}},
		{"first_last", "fl2va", []map[string]any{h3TestKeyframe(0), h3TestKeyframe(-1)}},
		{"image", "ref2va", []map[string]any{image}},
		{"mixed", "ref2va", []map[string]any{image, h3TestKeyframe(0)}},
		{"omni", "ref2va", []map[string]any{image, video, audio}},
		{"video_audio", "ref2va", []map[string]any{av}},
		{"audio_only_passthrough", "ref2va", []map[string]any{audio}},
	} {
		for _, explicit := range []bool{false, true} {
			name := tc.name + "/inferred"
			task := ""
			if explicit {
				name, task = tc.name+"/explicit", tc.task
			}
			t.Run(name, func(t *testing.T) {
				body := h3TestBody(t, task, tc.conditions)
				before := string(body)
				snapshot, normalized, err := prepareH3Request(body)
				if err != nil {
					t.Fatal(err)
				}
				if string(body) != before || snapshot.Seconds != 5 || snapshot.Resolution != "768p" {
					t.Fatal("original idempotency input or billing dimensions changed")
				}
				var input, output map[string]any
				if err = json.Unmarshal(body, &input); err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal(normalized, &output); err != nil {
					t.Fatal(err)
				}
				input["task"], input["duration"] = tc.task, float64(5)
				delete(input, "seconds")
				delete(input, "n")
				if !reflect.DeepEqual(input, output) {
					t.Fatal("normalization lost or reordered conditions/extension fields")
				}
			})
		}
	}
}

func TestH3ReferenceRejectsInvalidContract(t *testing.T) {
	for _, tc := range []struct{ name, fields string }{
		{"unknown_task", `"task":"i2v"`},
		{"text_with_input", `"task":"t2va","conditions":[{"type":"image","role":"reference","data":"data:image/png;base64,AQID"}]`},
		{"empty_keyframes", `"task":"fl2va"`},
		{"empty_references", `"task":"ref2va"`},
		{"reference_in_keyframe_task", `"task":"fl2va","conditions":[{"type":"image","role":"reference","data":"data:image/png;base64,AQID"}]`},
		{"keyframes_only_in_ref_task", `"task":"ref2va","conditions":[{"type":"image","role":"keyframe","frame_index":0,"data":"data:image/png;base64,AQID"}]`},
		{"missing_frame", `"conditions":[{"type":"image","role":"keyframe","data":"data:image/png;base64,AQID"}]`},
		{"null_frame", `"conditions":[{"type":"image","role":"keyframe","frame_index":null,"data":"data:image/png;base64,AQID"}]`},
		{"middle_frame", `"conditions":[{"type":"image","role":"keyframe","frame_index":1,"data":"data:image/png;base64,AQID"}]`},
		{"unknown_role", `"conditions":[{"type":"image","role":"first_frame","data":"data:image/png;base64,AQID"}]`},
		{"unknown_type", `"conditions":[{"type":"voice","role":"reference","data":"data:audio/wav;base64,AQID"}]`},
		{"ref_frame", `"conditions":[{"type":"image","role":"reference","frame_index":0,"data":"data:image/png;base64,AQID"}]`},
		{"foreign_content", `"content":[]`},
		{"foreign_voice_id", `"voice_id":"speaker-1"`},
		{"bad_quality", `"quality":"high"`},
		{"bad_upscale", `"upscale":"4x"`},
		{"bad_aspect", `"aspect_ratio":"2:1"`},
		{"conditions_object", `"conditions":{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"model":"MiniMax-H3","prompt":"waves",` + tc.fields + `}`)
			if _, _, err := prepareH3Request(body); err == nil {
				t.Fatal("accepted invalid contract")
			}
		})
	}
	duplicate := h3TestBody(t, "", []map[string]any{h3TestKeyframe(0), h3TestKeyframe(0)})
	if _, err := ParseH3Request(duplicate); err == nil {
		t.Fatal("accepted duplicate keyframe")
	}
}

func TestH3ReferenceCountLimits(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		images, videos, audios, av, frames int
		valid                              bool
	}{
		{"nine_images", 9, 0, 0, 0, 0, true},
		{"ten_images", 10, 0, 0, 0, 0, false},
		{"three_videos", 0, 3, 0, 0, 0, true},
		{"four_videos", 0, 4, 0, 0, 0, false},
		{"three_audios", 0, 0, 3, 0, 0, true},
		{"four_audios", 0, 0, 4, 0, 0, false},
		{"twelve_files", 8, 1, 1, 0, 2, true},
		{"thirteen_files", 9, 1, 1, 0, 2, false},
		{"three_av_files", 0, 0, 0, 3, 0, true},
		{"av_counts_as_audio", 0, 0, 1, 3, 0, false},
		{"av_counts_as_video", 0, 1, 0, 3, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var conditions []map[string]any
			for i := 0; i < tc.images; i++ {
				conditions = append(conditions, h3TestReference("image", "image/png"))
			}
			for i := 0; i < tc.videos; i++ {
				conditions = append(conditions, h3TestReference("video", "video/mp4"))
			}
			for i := 0; i < tc.audios; i++ {
				conditions = append(conditions, h3TestReference("audio", "audio/wav"))
			}
			for i := 0; i < tc.av; i++ {
				conditions = append(conditions, h3TestReference("video_audio", "video/mp4"))
			}
			for i := 0; i < tc.frames; i++ {
				conditions = append(conditions, h3TestKeyframe(-i))
			}
			_, err := ParseH3Request(h3TestBody(t, "ref2va", conditions))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}

func TestH3ReferenceDataURI(t *testing.T) {
	for _, tc := range []struct{ name, kind, data string }{
		{"url", "image", "https://example.org/first.png"},
		{"path", "image", "/tmp/first.png"},
		{"empty", "image", "data:image/png;base64,"},
		{"malformed", "image", "data:image/png;base64,!!!!"},
		{"truncated", "image", "data:image/png;base64,AQ"},
		{"trailing_data", "image", "data:image/png;base64,AQ==junk"},
		{"wrong_mime", "image", "data:audio/wav;base64,AQID"},
		{"no_base64", "image", "data:image/png,AQID"},
		{"video_audio_mime", "video_audio", "data:audio/wav;base64,AQID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			condition := map[string]any{"type": tc.kind, "role": "reference", "data": tc.data}
			_, err := ParseH3Request(h3TestBody(t, "ref2va", []map[string]any{condition}))
			if err == nil {
				t.Fatal("accepted invalid media")
			}
			if strings.Contains(err.Error(), tc.data) {
				t.Fatal("error leaked media input")
			}
		})
	}
}

func TestH3ReferenceByteLimits(t *testing.T) {
	// Exact 8 MiB decoded media is allowed even though JSON exceeds the old 2 MiB cap.
	encoded := base64.StdEncoding.EncodeToString(make([]byte, H3MaxMediaBytes))
	large := h3TestReference("image", "image/png")
	large["data"] = "data:image/png;base64," + encoded
	body := h3TestBody(t, "ref2va", []map[string]any{large})
	if len(body) <= 2<<20 || len(body) >= H3MaxRequestBytes {
		t.Fatal("fixture does not exercise media ceiling")
	}
	if _, err := ParseH3Request(body); err != nil {
		t.Fatalf("exact media ceiling: %v", err)
	}
	// Check cumulative bytes, not just a per-file limit (keyframes count too).
	body = h3TestBody(t, "ref2va", []map[string]any{large, h3TestKeyframe(0)})
	if _, err := ParseH3Request(body); err == nil || !strings.Contains(err.Error(), "8 MiB") {
		t.Fatalf("accepted cumulative media overflow: %v", err)
	}
	// Trailing JSON whitespace is valid; it also counts toward the transport cap.
	body = []byte(`{"model":"MiniMax-H3","prompt":"waves"}`)
	body = append(body, []byte(strings.Repeat(" ", H3MaxRequestBytes-len(body)))...)
	if _, err := ParseH3Request(body); err != nil {
		t.Fatalf("exact JSON ceiling: %v", err)
	}
	body = append(body, ' ')
	if _, err := ParseH3Request(body); err == nil || !strings.Contains(err.Error(), "12 MiB") {
		t.Fatalf("accepted JSON overflow: %v", err)
	}
}

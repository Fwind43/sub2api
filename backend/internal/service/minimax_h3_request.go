package service

// GeToken H3 uses task + conditions, not MiniMax's hosted content[] API.
// Keep the original conditions (including ordering and extension fields) intact:
// Picture/Video/Audio prompt references depend on their order.
import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// 8 MiB of decoded media needs ~10.67 MiB as base64, plus JSON/prompt.
	H3MaxRequestBytes = 12 << 20
	H3MaxMediaBytes   = 8 << 20
	h3MaxConditions   = 12
)

type h3Condition struct {
	Type       string          `json:"type"`
	Role       string          `json:"role"`
	FrameIndex json.RawMessage `json:"frame_index"`
	Data       string          `json:"data"`
}

type h3ReferenceRequest struct {
	Task        string          `json:"task"`
	Conditions  []h3Condition   `json:"conditions"`
	AspectRatio string          `json:"aspect_ratio"`
	Quality     string          `json:"quality"`
	Upscale     string          `json:"upscale"`
	Content     json.RawMessage `json:"content"`
	VoiceID     json.RawMessage `json:"voice_id"`
}

// validateH3References returns an explicit upstream task. Inference only applies
// when task is omitted; never silently change an explicitly requested mode.
// Media duration, image decodability and video audio-track presence are checked
// by GeToken's decoder, not trusted from optional client-supplied metadata.
func validateH3References(body []byte) (string, error) {
	var req h3ReferenceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return "", errors.New("invalid H3 reference parameters")
	}
	if len(req.Content) != 0 || len(req.VoiceID) != 0 {
		return "", errors.New("GeToken H3 uses conditions, not content or voice_id; use ref2va with an audio reference for voice timbre")
	}
	if req.Task != "" && req.Task != "t2va" && req.Task != "fl2va" && req.Task != "ref2va" {
		return "", errors.New("task must be t2va, fl2va or ref2va")
	}
	if !h3OneOf(req.AspectRatio, "", "16:9", "9:16", "1:1", "4:3", "3:4", "21:9") {
		return "", errors.New("unsupported H3 aspect_ratio")
	}
	if !h3OneOf(req.Quality, "", "draft", "standard", "final") {
		return "", errors.New("quality must be draft, standard or final")
	}
	if !h3OneOf(req.Upscale, "", "none", "realesrgan-x2") {
		return "", errors.New("upscale must be none or realesrgan-x2")
	}
	if len(req.Conditions) > h3MaxConditions {
		return "", errors.New("H3 accepts at most 12 input files")
	}
	keyframes := make(map[int]bool, 2)
	var images, videos, audios, references int
	var total int64
	for i, condition := range req.Conditions {
		fail := func(message string) (string, error) {
			return "", fmt.Errorf("conditions[%d]: %s", i, message)
		}
		if !h3OneOf(condition.Type, "image", "video", "video_audio", "audio") {
			return fail("type must be image, video, video_audio or audio")
		}
		switch condition.Role {
		case "keyframe":
			var frame int
			if condition.Type != "image" || len(condition.FrameIndex) == 0 || string(condition.FrameIndex) == "null" ||
				json.Unmarshal(condition.FrameIndex, &frame) != nil || (frame != 0 && frame != -1) {
				return fail("keyframe requires an image and frame_index 0 (first) or -1 (last)")
			}
			if keyframes[frame] {
				return fail("duplicate keyframe position")
			}
			keyframes[frame] = true
		case "reference":
			if len(condition.FrameIndex) != 0 {
				return fail("frame_index is only valid for keyframes")
			}
			references++
			switch condition.Type {
			case "image":
				images++
			case "video":
				videos++
			case "video_audio":
				videos++
				audios++
			case "audio":
				audios++
			}
		default:
			return fail("role must be keyframe or reference")
		}
		n, err := h3MediaSize(condition.Type, condition.Data, H3MaxMediaBytes-total)
		if err != nil {
			return fail(err.Error())
		}
		total += n
	}
	if images > 9 || videos > 3 || audios > 3 {
		return "", errors.New("H3 accepts at most 9 reference images, 3 videos and 3 audios; video_audio counts toward both video and audio limits")
	}
	task := req.Task
	if task == "" {
		switch {
		case references > 0:
			task = "ref2va"
		case len(keyframes) > 0:
			task = "fl2va"
		default:
			task = "t2va"
		}
	}
	switch task {
	case "t2va":
		if len(req.Conditions) != 0 {
			return "", errors.New("t2va does not accept conditions; use fl2va for keyframes or ref2va for references")
		}
	case "fl2va":
		if len(keyframes) == 0 || references != 0 {
			return "", errors.New("fl2va requires one or two image keyframes and no references; use ref2va for mixed input")
		}
	case "ref2va":
		if references == 0 {
			return "", errors.New("ref2va requires at least one reference; keyframes alone use fl2va")
		}
	}
	return task, nil
}

func h3OneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// Decode into io.Discard to validate base64 and count actual bytes without
// retaining another full copy of the media. Never fetch URLs or read paths.
func h3MediaSize(kind, data string, remaining int64) (int64, error) {
	header, encoded, ok := strings.Cut(data, ",")
	if !ok || !strings.HasPrefix(header, "data:") || !strings.HasSuffix(header, ";base64") || encoded == "" {
		return 0, errors.New("data must be a non-empty base64 data URI; URLs and local paths are not supported")
	}
	mime := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	valid := (kind == "image" && h3OneOf(mime, "image/png", "image/jpeg")) ||
		((kind == "video" || kind == "video_audio") && mime == "video/mp4") ||
		(kind == "audio" && h3OneOf(mime, "audio/wav", "audio/x-wav", "audio/mpeg", "audio/mp3"))
	if !valid {
		return 0, errors.New("data URI MIME must match type (PNG/JPEG image, MP4 video, WAV/MP3 audio)")
	}
	reader := base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(encoded))
	n, err := io.Copy(io.Discard, io.LimitReader(reader, remaining+1))
	if n > remaining {
		return 0, errors.New("all decoded H3 input files together must be at most 8 MiB")
	}
	if err != nil || n == 0 {
		return 0, errors.New("data contains invalid or empty base64 media")
	}
	return n, nil
}

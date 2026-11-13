# MiniMax H3: GeToken native reference requests

This gateway uses GeToken's `task` + `conditions` contract, **not** the hosted
MiniMax `content[]` / `voice_id` contract. Protocol reference:
<https://www.getoken.tech/dashboard/tokens> (H3 examples).

These are request-adapter capabilities. The reference modes described below have
not been verified by paid generation as part of this change. Previous delivery
verification covered only text-to-video, 5 seconds, native 768p.

## Endpoints and mode selection

- Create: `POST /v1/minimax/videos`
- Status: `GET /v1/minimax/videos/{id}`
- Content: `GET /v1/minimax/videos/{id}/content`
- Model: exactly `MiniMax-H3`; a MiniMax target group with media permission is required.

| `task` | Input |
| --- | --- |
| `t2va` | Text only; no conditions |
| `fl2va` | One or two image keyframes: first frame, last frame, or both |
| `ref2va` | Reference images, videos, audio, or video with audio; may include image keyframes |

When `task` is omitted/empty, the gateway infers `ref2va` if any reference is
present, otherwise `fl2va` if keyframes are present, otherwise `t2va`. An explicit
incompatible task is rejected rather than silently changed. Existing text-only
requests remain valid.

## Conditions

Each condition contains `type`, `role`, and `data`:

- `role: "keyframe"`: requires `type: "image"` and `frame_index: 0` (first) or
  `-1` (last). Each position may occur only once. A last-frame-only input retains
  `frame_index: -1`.
- `role: "reference"`: `type` is `image`, `video`, `audio`, or `video_audio`;
  omit `frame_index`. Use `video_audio` for an MP4 whose audio also provides a
  reference.
- `data`: base64 data URI of a local file. No external URL fetching or server-local
  paths. MIME types: PNG/JPEG image, MP4 video, WAV/MP3 audio.
- Ordering and extension fields are preserved when forwarding. Reference prompt
  tags follow the provider examples: `<Picture 1>`, `<Video 1>`, `<Audio 1>`.

Audio/voice-timbre reference uses `task: "ref2va"` with an audio condition and a
prompt describing how to use it. This is **not** a separate voice registration or
`voice_id` cloning API and does not guarantee identity-level voice matching.
The gateway does not impose an undocumented image requirement on audio-only
references; acceptance of actual media and resulting quality remain upstream concerns.
Use only media and voices you have permission to process.

## Limits and options

- Up to 9 reference images, 3 videos, 3 audios; `video_audio` counts toward both
  video and audio limits. All inputs, including keyframes, total at most 12 files.
- All decoded input files together: **8 MiB**. The JSON request cap is **12 MiB**
  to accommodate base64 overhead. Exceeding the HTTP cap returns 413; invalid
  conditions or decoded-media overflow return 400. A lower operator/proxy cap
  still applies; other gateway routes' limits are unchanged.
- Provider audio/video limits: each clip 2–15 seconds, each modality's total at
  most 15 seconds. Actual duration, decoding and track presence are checked by
  the upstream, not inferred from untrusted client metadata by this adapter.
- Output duration: 4–15 integer seconds; `seconds` is a compatibility alias.
  Conflicting nonzero `duration` / `seconds` values are rejected. Omission retains
  the existing 6-second default; specify explicitly for predictable billing.
- Use `resolution: "768p"` for GeToken native output. The gateway's previous
  resolution aliases/default are unchanged; acceptance is not proof that the
  upstream generates those nominal resolutions.
- `aspect_ratio`: `16:9`, `9:16`, `1:1`, `4:3`, `3:4`, `21:9`.
- `quality`: `draft`, `standard`, `final` (provider examples: 20/30/50 steps).
- `upscale`: `none`, `realesrgan-x2`; native output is not the same as upscaled
  dimensions. Existing generation/billing snapshots are not changed by this patch.
- `seed` and other extension fields are forwarded, not treated as verified capabilities.
- One video per task; the compatibility `n` field is removed before forwarding.

## Build a local request: first frame + image + voice reference

This script only prepares JSON. It does not submit or charge anything.

```python
import base64
import json
from pathlib import Path

input_bytes = 0

def media(filename):
    global input_bytes
    path = Path(filename)
    mime = {".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
            ".mp4": "video/mp4", ".wav": "audio/wav", ".mp3": "audio/mpeg"}[path.suffix.lower()]
    raw = path.read_bytes()
    input_bytes += len(raw)
    if input_bytes > 8 * 1024 * 1024:
        raise ValueError("All input media must total at most 8 MiB")
    return "data:" + mime + ";base64," + base64.b64encode(raw).decode("ascii")

payload = {
    "model": "MiniMax-H3",
    "task": "ref2va",
    "prompt": "Start from the given keyframe. Use <Picture 1> for appearance; "
              "use <Audio 1> as the voice-timbre reference for the spoken greeting.",
    "duration": 5,
    "resolution": "768p",
    "aspect_ratio": "16:9",
    "quality": "final",
    "upscale": "none",
    "conditions": [
        {"type": "image", "role": "keyframe", "frame_index": 0, "data": media("first.png")},
        {"type": "image", "role": "reference", "data": media("reference.jpg")},
        {"type": "audio", "role": "reference", "data": media("voice.wav")},
    ],
}
Path("video-request.json").write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")
```

For first/last-frame I2V, use `task: "fl2va"` and keep only image keyframes
(`frame_index` 0 and/or -1). For full multimodal reference, use `ref2va` and add
`{"type": "video", "role": "reference", "data": media("motion.mp4")}` or
`video_audio` when retaining the video's sound. Stay within all limits above.

## Submission and delivery safety

Submission may incur charges. After the caller authorizes the exact request:

1. Save the exact JSON bytes and one unique `Idempotency-Key` before submission.
2. POST that file once with `Authorization: Bearer <key>` and JSON content type.
3. Save the returned task `id` (not the HTTP request ID). Poll the status endpoint.
4. Download only after `status=completed`, `delivery_status=verified`, and
   `billing_status=settled`. Save the file and inspect its actual duration/dimensions.

Idempotency is based on the original request bytes, not normalized JSON.
Changing whitespace or explicitly adding an inferred task changes those bytes.
If submission is unknown, do not generate a new key or automatically create a
new job; reconcile the existing request first. Client timeouts do not imply
cancellation or non-billing. Do not re-generate when only download failed.

The provider states generated files are available for 24 hours; download promptly.
Input media, prompts, parameters and output may be retained in its private archive.
This patch adds no automatic refund, cancellation, local media decoding, voice
cloning guarantee, or change to the companion short-drama plugin's advertised modes.

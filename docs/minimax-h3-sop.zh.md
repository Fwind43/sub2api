# MiniMax H3 视频生成调用 SOP

> 面向通过 Sub2API 网关调用 MiniMax H3 的开发者、自动化脚本和 AI 助手。
> 核对基线：Sub2API fork `0.2.23.3`（tag 提交 `7129a82c4`）源码。已做过付费端到端验收的只有 **文生视频、5 秒、768p、单条视频**；i2v、首尾帧、全能参考、音色参考已上线，但尚无这些模式的付费端到端验收。
> 示例中的网关地址、Key 均为占位符；文中脚本是调用模板，按实际环境替换后再运行。

## 1. 先记住这条流程

```text
确认分组权限与费用
  → 保存原始请求文件 + 唯一幂等键
  → POST /v1/minimax/videos（一个业务任务只创建一次）
  → 立即保存返回的 h3-... 任务 ID
  → GET /v1/minimax/videos/{id}，约每 20 秒查询
  → status=completed 且 delivery_status=verified 且 billing_status=settled
  → GET /v1/minimax/videos/{id}/content，保存 MP4
```

**不要把 HTTP 202 当作生成成功；不要只看到 `completed` 就宣告交付；不要因超时而换幂等键重复创建。**

## 2. 调用前准备

| 项目 | 要求 |
| --- | --- |
| 网关地址 | 管理员提供的 HTTPS 地址。本文以 `https://gateway.example.com` 为例；其他部署替换为对应地址。地址不含末尾 `/v1`。 |
| API Key | 使用调用者自己的 **Sub2API API Key**，不是上游供应商 Key，也不是管理后台登录令牌。只通过环境变量或安全输入提供。 |
| 分组 | API Key 绑定 MiniMax 分组，或配置了 `MiniMax-H3 → minimax` 路由的复合分组；需管理员开通。无需调用者配置 Grok。 |
| 媒体权限 | 分组需允许图像/媒体生成；当前实现的权限开关为 `allow_image_generation`，视频也受其控制。 |
| 额度与定价 | 有效余额/订阅及额度；管理员已配置可用 MiniMax 账户和正数视频价格。示例费用不是对其他用户的报价。 |
| 敏感信息 | 不把 Key 写进 SOP、Git、截图、命令输出或提交给客服的日志。分享收据前移除 Authorization。 |

创建、查状态、下载均使用：

```http
Authorization: Bearer <你的Sub2API_API_Key>
```

**全流程使用同一把 Key，且不要在任务处理中切换该 Key 的绑定分组。** 查询权限同时关联用户、Key 和分组；同一用户换另一把 Key 也可能查询不到任务。不要把网关 Key 发给上游或响应中的其他域名。

## 3. 接口与参数

### 3.1 固定使用 MiniMax 独立接口

| 操作 | 方法与路径 | 正常响应 |
| --- | --- | --- |
| 创建任务 | `POST /v1/minimax/videos` | HTTP `202`，任务 JSON，`Location` 指向状态接口 |
| 查询状态 | `GET /v1/minimax/videos/{id}` | HTTP `200`，任务 JSON |
| 下载成片 | `GET /v1/minimax/videos/{id}/content` | HTTP `200`，`Content-Type: video/mp4`，二进制视频 |

这里的 `{id}` 是网关返回的 `h3-...`，不是上游任务 ID。不要改成上游的 `/v1/videos/generations`，也不要套用 Grok 的模型名或字段。本流程不使用 SSE、聊天接口或图片异步任务接口。

### 3.2 推荐请求

保存为 UTF-8 编码的 `request.json`，JSON 中不能带注释：

```json
{
  "model": "MiniMax-H3",
  "prompt": "电影级写实人像，傍晚柔和暖光中，一位成年女性缓缓转身回眸，目光自然地看向镜头，随后露出温柔而真诚的微笑。中近景构图，镜头轻柔推进，发丝随微风轻动，皮肤纹理自然，浅景深，人物五官稳定，动作连贯细腻，画面干净，无字幕，无水印。",
  "duration": 5,
  "resolution": "768p"
}
```

上面的提示词是可替换的示例，不代表画面内容保证。文生视频只需这些字段；**图生视频、参考图、宽高比、种子等扩展参数不在本 SOP 已验收范围内**。

| 字段 | 规则 |
| --- | --- |
| `model` | 必须精确为 `MiniMax-H3`，大小写敏感。 |
| `prompt` | 非空字符串。 |
| `duration` | JSON 整数。推荐显式传 `5`。网关校验允许 4–15 秒，不等于当前上游全部支持；自部署上游可能有更低上限（已遇到 15 秒请求在提交阶段被拒）。 |
| `resolution` | 推荐显式传 `"768p"`。网关可接受 `480p/720p/768p/1080p`，但本链路实际验收的是 `768p`。 |
| `seconds` | 兼容时长字段；建议不传。与 `duration` 同时出现时必须相同。 |
| `n` | 建议不传；如需传，只使用 `1`。一个任务只生成一条视频。 |

不要依赖默认值：省略时长会默认 6 秒，省略分辨率会默认 720p，这不是本 SOP 已成功验收的组合。本接口 JSON 上限为 **64 MiB**；所有输入素材解码后合计最多 **47 MiB**。外层代理或运营配置若更小，仍可能先拦截。

### 3.3 模式选择：文生、i2v、首尾帧、全能参考、音色参考

已部署适配使用 **GeToken 原生 `task` + `conditions`**，不是 MiniMax 托管接口。不要传 `content`（包括 `content: null`）或 `voice_id`；网关会拒绝。

| 场景 | `task` | `conditions` |
| --- | --- | --- |
| 文生视频 | `t2va` | 省略或空数组；不能附带素材 |
| i2v：首帧图生视频 | `fl2va` | 一个 `image/keyframe`，`frame_index: 0` |
| 仅尾帧约束 | `fl2va` | 一个 `image/keyframe`，`frame_index: -1` |
| 首尾帧控制 | `fl2va` | 两个 `image/keyframe`，分别为 `0`、`-1` |
| 图片/视频/音频全能参考 | `ref2va` | 至少一个 `role: reference`；可混合多种媒体 |
| 音色参考（包括仅音频输入） | `ref2va` | `type: audio, role: reference`，提示词描述音色用途 |
| 首帧 + 人物图 + 声音等混合输入 | `ref2va` | keyframe 与至少一个 reference 共存 |

省略或留空 `task` 时，网关按“存在 reference → `ref2va`；否则有 keyframe → `fl2va`；否则 → `t2va`”推断。建议显式填写；显式模式与素材不兼容时返回 400，不会悄悄改模式。音色参考不是声音注册、`voice_id` 克隆接口，不保证身份级声纹一致。

### 3.4 `conditions` 字段与素材限制

每个元素包含 `type`、`role`、`data`；keyframe 还必须有 `frame_index`。

```json
{
  "type": "image",
  "role": "keyframe",
  "frame_index": 0,
  "data": "data:image/jpeg;base64,<真实文件的Base64>"
}
```

上面仅展示结构，尖括号占位符不是合法素材。实际文件请用下节脚本编码。

- `role: keyframe`：仅接受 `type: image`；`frame_index` 必须为整数 `0` 或 `-1`，每个位置最多一次。
- `role: reference`：接受 `image`、`video`、`audio`、`video_audio`；**不要填写 `frame_index`**，包括 `null`。`video_audio` 用于同时参考画面与声音的 MP4。
- `data` 必须是文件的 **Base64 data URI**，不是裸 Base64、HTTP URL、本地路径或 multipart 文件。
- MIME：图片 `image/png`、`image/jpeg`；视频 `video/mp4`；音频 `audio/wav`、`audio/x-wav`、`audio/mpeg`、`audio/mp3`。MIME 必须与 `type` 对应，不能只改扩展名冒充格式。
- 最多 **9 张参考图、3 段视频、3 段音频**；`video_audio` 同时占视频和音频配额；包含首尾帧在内，总计最多 **12 个文件**。
- 全部原始媒体合计最多 **47 MiB = 49,283,072 字节**；完整 JSON 最多 **64 MiB = 67,108,864 字节**。Base64 会膨胀体积，不要把 47 MiB 当作每个文件限制。
- 供应商文档要求音频/视频单段 **2–15 秒**，同一媒体类别合计最多 **15 秒**；实际解码、时长、是否有音轨由上游检查，网关并未做本地媒体验真。带声音视频应保守地同时计入音频和视频时长预算。
- 素材顺序与扩展字段会保留。提示词按供应商方式引用 `<Picture 1>`、`<Video 1>`、`<Audio 1>`，保持同类素材顺序稳定，不在提交后重新排序。编号的实际解释及参考效果由上游决定。
- 只能上传有权处理的图片、视频和声音，特别是肖像与音色。供应商文档说明输入媒体、提示词、参数和输出可能保存在其私有归档；不要上传未获准外传的敏感内容。

### 3.5 画幅、质量与放大选项

| 字段 | 可接受值 | 注意 |
| --- | --- | --- |
| `aspect_ratio` | `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9` | 竖屏可选 `9:16`；这是输出请求，不证明任意组合已实测 |
| `quality` | `draft`、`standard`、`final` | 供应商示例对应 20/30/50 steps；不是质量或耗时承诺 |
| `upscale` | `none`、`realesrgan-x2` | 原生 768p 与后处理放大不同；账单标签不保证等于实际交付尺寸 |
| `seed` 等扩展字段 | 透传 | 网关透传不代表上游必支持、效果可复现或已验收 |

上述三个枚举字段可省略，网关不替调用方承诺供应商默认值。需要明确规格时显式填写。一个任务只生成一条视频；`seconds`、`n` 是兼容字段，网关规范化时移除，并补入 `duration`、`resolution`、`task`。**幂等比较仍基于入站原始字节，不是规范化后的 JSON。**

### 3.6 用本地文件构造请求：七种场景共用脚本

下例只在本地生成 `request.json`，**不联网、不提交、不扣费**。保存为 `build_request.py`，在新的任务目录执行。脚本示例未在本次文档整理中运行；不会解码媒体或检查真实时长，提交前仍需自行确认。

```python
import argparse
import base64
import json
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("mode", choices=[
    "t2v", "i2v", "last", "first-last", "reference", "voice", "mixed"
])
args = parser.parse_args()

# 文件名、type、role、frame_index（reference 无此字段）
first = ("first.jpg", "image", "keyframe", 0)
last = ("last.jpg", "image", "keyframe", -1)
picture = ("character.png", "image", "reference", None)
video = ("motion.mp4", "video", "reference", None)
audio = ("voice.wav", "audio", "reference", None)
modes = {
    "t2v": ("t2va", "A calm cinematic shot of a sunlit forest.", []),
    "i2v": ("fl2va", "Animate from the supplied first frame; slow camera push-in.", [first]),
    "last": ("fl2va", "Create a smooth shot ending at the supplied last frame.", [last]),
    "first-last": ("fl2va", "Smoothly transition from the first frame to the last frame.", [first, last]),
    "reference": ("ref2va", "Use <Picture 1> for appearance, <Video 1> for motion, and <Audio 1> for vocal timbre.", [picture, video, audio]),
    "voice": ("ref2va", 'A person says "Hello" using the vocal timbre of <Audio 1>.', [audio]),
    "mixed": ("ref2va", "Begin at the keyframe; use the reference image for appearance and <Audio 1> for vocal timbre.", [first, picture, audio]),
}
task, prompt, specs = modes[args.mode]
mimes = {
    ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
    ".mp4": "video/mp4", ".wav": "audio/wav", ".mp3": "audio/mpeg",
}
allowed = {
    "image": {"image/png", "image/jpeg"},
    "video": {"video/mp4"}, "video_audio": {"video/mp4"},
    "audio": {"audio/wav", "audio/mpeg"},
}
conditions = []
total = 0
for filename, kind, role, frame in specs:
    path = Path(filename)
    mime = mimes.get(path.suffix.lower())
    if mime not in allowed[kind]:
        raise ValueError(f"Unsupported media type: {path}")
    with path.open("rb") as f:
        raw = f.read(47 * 1024 * 1024 - total + 1)
    total += len(raw)
    if not raw or total > 47 * 1024 * 1024:
        raise ValueError("Empty file or total decoded media exceeds 47 MiB")
    condition = {
        "type": kind, "role": role,
        "data": "data:" + mime + ";base64," + base64.b64encode(raw).decode("ascii"),
    }
    if role == "keyframe":
        condition["frame_index"] = frame
    conditions.append(condition)

payload = {
    "model": "MiniMax-H3", "task": task, "prompt": prompt,
    "duration": 5, "resolution": "768p",
    "aspect_ratio": "16:9", "quality": "standard", "upscale": "none",
}
if conditions:
    payload["conditions"] = conditions
body = json.dumps(payload, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
if len(body) > 64 * 1024 * 1024:
    raise ValueError("JSON exceeds 64 MiB")
# x 模式防止覆盖原始请求；已有请求不要删除后重新生成来重试。
with Path("request.json").open("xb") as f:
    f.write(body)
print(f"Prepared request.json: {len(body)} bytes; task={task}; no API call made")
```

每次只选一个命令；不要在同一任务目录依次执行所有模式：

```bash
python3 build_request.py t2v         # 无输入文件
python3 build_request.py i2v         # first.jpg
python3 build_request.py last        # last.jpg
python3 build_request.py first-last  # first.jpg + last.jpg
python3 build_request.py reference   # character.png + motion.mp4 + voice.wav
python3 build_request.py voice       # voice.wav；网关不额外强制参考图
python3 build_request.py mixed       # first.jpg + character.png + voice.wav
```

Windows 可使用已正确安装的 `py -3` 或 `python` 替换 `python3`。修改脚本内提示词、输出画幅及文件名应在首次生成请求之前完成。如果 MP4 的声音也需参考，把对应规格的 `video` 改成 `video_audio`，不要额外编造上传接口或 `voice_id`。

### 3.7 提交前检查

1. 明确本次费用授权、所属分组与规格；不要把已部署适配等同于参考效果已验收。
2. 选择一种模式，确认 `conditions` 的角色、数量、顺序和 MIME；素材真实可解码，时长符合供应商要求。
3. 确认 `request.json` 是 UTF-8 无 BOM，模型名准确，显式给出 `duration`、`resolution`。
4. 保存原始请求及唯一幂等键；首次 POST 后绝不改 JSON、换 Key/分组或换幂等键来重试。
5. 开始下面的创建 → 轮询 → 下载流程。原始请求可能含完整 Base64 素材，应按敏感文件保管。

## 4. cURL 完整操作示例

适用于 Bash / Git Bash / WSL，需 `curl`、`jq`、`python3`。Windows 原生 PowerShell 用户见第 5 节。

### 4.1 首次准备：一个目录对应一个业务任务

创建一个新的工作目录后，放入上面的 `request.json`。不要在已有任务目录覆盖它。

```bash
export BASE_URL="https://gateway.example.com"
read -r -s -p "Sub2API API Key: " SUB2API_API_KEY; echo
export SUB2API_API_KEY

# 仅首次创建本地幂等键；恢复已有任务时直接复用文件。
if [ ! -s idempotency-key.txt ]; then
  python3 -c 'import uuid; print(uuid.uuid4())' > idempotency-key.txt
fi
IDEM="$(cat idempotency-key.txt)"
```

保存好 `request.json`、`idempotency-key.txt`。服务器根据**原始请求字节**计算幂等摘要：重排 JSON 字段、修改缩进、编码、换行或提示词，都可能造成同键冲突。重试时继续用原文件，不要重新序列化。

### 4.2 创建一次并保存任务 ID

**以下 POST 可能产生费用。必须先得到调用者对本次生成的授权。** 不要给 POST 添加 `--retry`，不要用定时器反复执行整段创建逻辑。

```bash
# 已有任务时不再 POST；幂等键缺失时停下，不临时换新键。
if [ -s task-id.txt ]; then
  echo '已有任务，请直接查询状态。'
elif [ ! -s idempotency-key.txt ] || [ -z "$IDEM" ]; then
  echo '缺少幂等键，停止创建并核查本地记录。'
else
  HTTP_CODE="$(curl --silent --show-error --max-time 60 \
    --request POST "$BASE_URL/v1/minimax/videos" \
    --header "Authorization: Bearer $SUB2API_API_KEY" \
    --header 'Content-Type: application/json' \
    --header "Idempotency-Key: $IDEM" \
    --data-binary @request.json \
    --dump-header create.headers \
    --output create.json \
    --write-out '%{http_code}')"
  CURL_RC=$?
  if [ "$CURL_RC" -ne 0 ] || [ "$HTTP_CODE" != "202" ]; then
    echo "创建结果需核查：curl=$CURL_RC HTTP=$HTTP_CODE；保留原请求和幂等键，不要换键重建。"
  else
    TASK_ID="$(jq -er '.id | select(type == "string" and startswith("h3-"))' create.json)" &&
    printf '%s\n' "$TASK_ID" > task-id.txt
  fi
fi
```

`202` 的 JSON 直接包含 `id`、`status`、`delivery_status`、`billing_status`、`created_at`，没有外层 `data` 包装。首次通常处于 `submitting`，也可能已推进到后续状态。

如果没有得到合法任务 ID，**不要继续下载或自动新建任务**，按第 7 节处理。尤其不要把 HTTP 响应头中的 `X-Request-Id` 误当作视频任务 ID。

### 4.3 查询状态，不再 POST

```bash
TASK_ID="$(cat task-id.txt)"
if curl --silent --show-error --fail-with-body --max-time 60 \
  "$BASE_URL/v1/minimax/videos/$TASK_ID" \
  --header "Authorization: Bearer $SUB2API_API_KEY" \
  --output status.json; then
  jq . status.json
else
  echo '本次查询失败，不能用旧状态判断成功；保留任务 ID，稍后继续 GET。'
fi
```

建议每 20 秒执行一次状态 GET；网络错误可退避至 30–60 秒。轮询不代表重新创建任务。客户端等待上限可自行设置为 40 分钟；到时仅停止等待并保留任务 ID，不等于取消任务，也不代表它不会稍后完成并结算。

**最终成功示意**（ID/时间为占位）：

```json
{
  "id": "h3-<任务UUID>",
  "status": "completed",
  "delivery_status": "verified",
  "billing_status": "settled",
  "created_at": "<RFC3339时间>"
}
```

三个状态都满足后才能进入下载步骤。`submission_unknown` 必须停止自动创建并交管理员核查；其他状态见第 6 节。

### 4.4 下载 MP4

仅在三个成功状态均满足时执行；`video.part` 用于避免把错误 JSON 当成 MP4 成品。

```bash
HTTP_CODE="$(curl --silent --show-error --max-time 300 \
  "$BASE_URL/v1/minimax/videos/$TASK_ID/content" \
  --header "Authorization: Bearer $SUB2API_API_KEY" \
  --dump-header download.headers \
  --output video.part \
  --write-out '%{http_code}')"
CURL_RC=$?
if [ "$CURL_RC" -eq 0 ] && [ "$HTTP_CODE" = "200" ] && \
   grep -qi '^content-type: video/mp4' download.headers; then
  mv video.part video.mp4
  echo '视频已保存：video.mp4'
else
  echo "下载未完成：curl=$CURL_RC HTTP=$HTTP_CODE；保留任务 ID，仅重试下载。"
fi
```

不要对带 Authorization 的请求盲目添加跨域跳转选项，也不要从 JSON 中抓取任意视频 URL 后附带网关 Key 请求它。请使用上述固定网关 `/content` 接口。

下载后按需播放检查；有 FFmpeg 时可检查媒体规格（此操作不会重新生成）：

```bash
ffprobe -v error -show_entries stream=codec_name,width,height:format=duration,size -of json video.mp4
```

实际封装时长和尺寸可能与名义参数略有差异；成功样例为 1344×768、5.175 秒，而请求时长为 5 秒。内容下载应尽快完成并自行保存，不把网关当作永久媒体存储服务。

## 5. Windows PowerShell 调用方式

仍先保存 `request.json`，使用同一任务目录。下例使用系统 HTTP 客户端，不依赖 `jq`。创建段只运行一次；恢复任务时只运行查询/下载段。

```powershell
$BaseUrl = 'https://gateway.example.com'
$SecureKey = Read-Host 'Sub2API API Key' -AsSecureString
$PlainKey = [System.Net.NetworkCredential]::new('', $SecureKey).Password
$Headers = @{ Authorization = "Bearer $PlainKey" }

if (-not (Test-Path 'idempotency-key.txt')) {
    [IO.File]::WriteAllText((Join-Path $PWD 'idempotency-key.txt'), [guid]::NewGuid().ToString())
}
$Idem = (Get-Content -Raw 'idempotency-key.txt').Trim()
$CreateHeaders = @{ Authorization = "Bearer $PlainKey"; 'Idempotency-Key' = $Idem }

# 已有任务时跳过 POST；异常时不要换键或覆盖 request.json。
if (-not (Test-Path 'task-id.txt')) {
    if ([string]::IsNullOrWhiteSpace($Idem)) { throw '缺少幂等键，停止创建' }
    $Task = Invoke-RestMethod -Method Post -Uri "$BaseUrl/v1/minimax/videos" -Headers $CreateHeaders -ContentType 'application/json' -InFile 'request.json' -TimeoutSec 60 -ErrorAction Stop
    $Task | ConvertTo-Json -Depth 10 | Set-Content -Encoding UTF8 'create.json'
    if ($Task.id -notlike 'h3-*') { throw '缺少有效任务 ID，请保留收据并核查' }
    [IO.File]::WriteAllText((Join-Path $PWD 'task-id.txt'), $Task.id)
}

# 后续只查状态；建议每 20 秒一次，不要重新执行上面的 POST。
$TaskId = (Get-Content -Raw 'task-id.txt').Trim()
$Status = Invoke-RestMethod -Method Get -Uri "$BaseUrl/v1/minimax/videos/$TaskId" -Headers $Headers -TimeoutSec 60 -ErrorAction Stop
$Status | ConvertTo-Json -Depth 10

if ($Status.status -eq 'completed' -and $Status.delivery_status -eq 'verified' -and $Status.billing_status -eq 'settled') {
    $Response = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/v1/minimax/videos/$TaskId/content" -Headers $Headers -OutFile 'video.part' -PassThru -TimeoutSec 300 -ErrorAction Stop
    if ($Response.StatusCode -eq 200 -and "$($Response.Headers['Content-Type'])" -like 'video/mp4*') {
        Move-Item 'video.part' 'video.mp4'
    } else {
        throw '响应不是已确认的 MP4，保留任务 ID，仅重试下载'
    }
}
```

任务完成后可关闭当前终端，避免 Key 长期留在会话变量中。不要在共享终端中输出 `$Headers`、`$PlainKey` 或启动包含明文密钥的转录日志。

## 6. 状态解释与计费边界

| 状态/组合 | 含义与动作 |
| --- | --- |
| `status=submitting` | 已在网关持久化，后台正提交上游。继续 GET；不是让客户端再次 POST。上游提交有最长 30 分钟的后台超时，不是客户端要保持 POST 连接 30 分钟。 |
| `status=pending` | 等待生成结果，继续 GET。 |
| `status=completed`，另外两项未齐 | 上游已完成，但网关仍需验证媒体或完成结算；不能直接视为可交付。 |
| `completed + verified + settled` | 网关已验证 MP4 并完成结算，可以下载；仍应确认本地下载成功。 |
| `status=failed`、`billing_status=not_billable` | 网关判定失败且不计入该任务账单；停止轮询。若确需重新生成，先确认失败原因与本次费用授权，再创建新的业务任务。 |
| `status=submission_unknown` | 上游是否受理存在歧义，不等于确定失败、不等于未产生上游费用；停止自动重建，交管理员核查。 |
| 长期无变化、陌生状态或身份/分组变更 | 保存任务 ID、时间和响应，联系管理员；不要靠换 Key、换分组、换幂等键绕过。 |

计费说明：

- 任务创建时冻结价格与规格；正常流程是网关获取并验证 MP4 后调用账务结算，而不是在客户端点击下载时结算。
- 因此，客户端停止等待或下载网络中断，不会自动取消后台任务，也不保证不扣费。
- `/content` 下载失败可能返回 `502 h3_delivery_error`，提示任务结算状态不变；应重试同一任务的下载或联系管理员，不要重新生成。
- 成功验收样例中，同一任务两次下载内容一致，且账务仅一条用量记录、一次扣费。该样例为基础费用 `$2.50 × 0.3 = $0.75`，**不是所有调用者的固定价格或未来报价**；具体按分组/用户实际定价确认。
- 当前公开任务 JSON 不返回费用明细或自动退款承诺；请在用量/账单页面核对，异常时交管理员对账。不要自行删账单、删去重记录或盲目重新扣费。

## 7. 幂等、重试与错误排查

### 幂等的正确用法

1. 每个真正的新视频需求生成一个唯一 `Idempotency-Key`，长度不超过 200。
2. 在第一次发送前，持久保存幂等键和**完整原始请求文件**。
3. 已拿到任务 ID 后，后续只 GET 状态/内容，不再 POST。
4. POST 超时、断线、502 且没拿到任务 ID：保存现场，确认 Key/分组未变后，必要时仅以**原 Key + 原幂等键 + 原字节请求**做受控重试。已有任务会返回原任务，不会用该键重新提交上游；该重试也可能遇到鉴权/额度检查，不保证总能恢复 ID。
5. `submission_unknown` 或 `failed` 的旧任务不会因为复用幂等键就重新生成。新幂等键是一个新业务任务，可能新增费用，必须单独确认。
6. 如果第一次没传幂等键且响应丢失，客户端没有可靠的去重恢复依据；先找管理员，不要自动补发。

| 现象 | 建议 |
| --- | --- |
| `400` 参数错误 | 检查精确模型名、JSON、非空提示词、整数时长、`duration/seconds` 是否冲突；确认使用独立路径。 |
| `400` 参考参数错误 | 检查 task 与 conditions 是否匹配、role/type/frame_index、Base64/MIME、素材数量和解码后总大小；删除不支持的 `content`/`voice_id`。不要用同一幂等键提交修改后的请求来试错。 |
| `400 MiniMax target group required` | 管理员检查 Key 分组和复合路由目标是否为 `minimax`。 |
| `401/403` | 检查 Key、权限、媒体生成开关及服务返回的额度/风控信息；不要转发真实 Key 给他人排查。 |
| `409 idempotency_conflict`（创建） | 相同幂等键对应的原始请求字节或身份/分组不同。恢复原文件和原身份；不要为了消除错误随手换键。 |
| `404 H3 task not found`（查询/下载） | 检查网关域名、`h3-...` ID、同一 Key 及原绑定分组。不能据此断言原任务不存在或从未计费。 |
| `409 video_not_ready`（下载） | 尚未 verified + settled，响应给出 `Retry-After: 20`；先继续查状态。 |
| `413` 请求过大 | JSON 超过 64 MiB 或更小的代理/运营限额；全部媒体解码后超过 47 MiB 则由参数校验返回 400。确认原请求未受理后再准备较小素材的新业务请求，不自动换键重发。 |
| `429` | 按响应提示退避，检查并发/配额；不得改键高频重建。 |
| `502 h3_error`（创建） | 保留响应与 `X-Request-Id`，按上面的原键原请求规则恢复；需要时交管理员查创建阶段。 |
| `502 h3_delivery_error`（下载） | 已有结算状态不变；仅重试该任务的下载，持续失败则联系管理员处理。 |
| 创建请求直接返回 HTTP `503` | 先查入口代理、网关和持久化链路；不能仅凭此响应断定 H3 上游返回了 503。保留响应与请求追踪号，不自动重建任务。 |
| 已有任务为 `submission_unknown`，且后台提交日志记录上游 HTTP `503` | 网关已建任务，故障出现在后台提交阶段。按任务 ID 与提交时间查提交日志并交管理员对账；上游是否受理或计费仍不明，不换幂等键重发。 |

管理员只读对账：

- 用 GeToken 用量日志的 `createdAt - latencyMs` 回推请求发起时间，再对照网关 `minimax_h3_tasks.created_at` 与提交日志，缩小候选范围。并发时不要按时间或完成顺序认领；须有共用请求号或上游任务 ID 才能逐笔确认。上游成功、网关交付和本站结算分别核对。
- 提交日志的 `upstream_http_status=0` 是未记录到上游 HTTP 错误码，不是实际响应码，不能据此断定请求超时、失败或未计费。

参数修正属于新请求，不是原字节重试。必须先明确旧请求是否已被接受；存在歧义时交管理员确认，不能通过不断更换幂等键来试参数。

当前独立接口未提供本 SOP 可用的取消接口，也不提供按幂等键查询任务的公开 GET 接口。**请求已受理后退出脚本，不等于取消生成。**

## 8. 交付与问题反馈清单

调用成功后应交付实际 MP4 文件，而不是仅交付任务 ID 或一段“成功”文字。建议保留：

- 任务 ID、创建时间、网关域名（不含 Key）；
- 原始 `request.json` 和幂等键：内部留存，注意提示词及 Base64 媒体可能包含隐私；
- `create.json`、最终状态 JSON、HTTP 状态与 `X-Request-Id`；
- MP4 文件大小、可选 SHA256、分辨率与时长；
- 如有争议，提供脱敏用量记录及扣费时间。

供应商文档声明生成文件可用期为 **24 小时**，应尽快下载并自行保存。这不是网关永久保管或至少可下载 24 小时的额外保证；本次未重新验证过期行为。

不要把 API Key、Authorization、上游凭据、管理后台令牌或整份敏感请求日志作为附件对外发送。状态/下载端点需要鉴权，不能把它们当作无需授权的公开分享链接。给管理员报错优先提供任务 ID、错误码和脱敏响应，不要先发送全部素材。

### 可复制给 AI 助手的执行约束

```text
使用 MiniMax H3 独立网关接口，不使用 Grok 路由或 MiniMax 托管 content/voice_id 协议。
先确认调用地址、具备 MiniMax 权限的 Key，以及本次付费生成授权。
默认建议 model=MiniMax-H3、duration=5、resolution=768p，单条视频。
根据需求选择 task=t2va/fl2va/ref2va；参考素材使用 conditions 和 base64 data URI。
按顺序保留参考素材；keyframe 仅图像且 frame_index=0/-1，reference 不带 frame_index。
全部媒体合计<=47MiB、JSON<=64MiB；核查数量、MIME、真实时长和素材使用授权。
发送前保存唯一幂等键与原始请求字节；只创建一次，保存 h3- 任务 ID。
全流程使用同一 Key 和原绑定分组；每20秒查状态，completed+verified+settled 后下载 MP4。
超时不换键重建；submission_unknown 停止自动重试并请求人工核查。
下载失败只重试原任务下载，不触发新生成；不自动退款或改账。
交付实际文件，报告真实规格、状态及验证边界；不得输出密钥或敏感素材。
参考适配已部署，但本SOP没有这些参考模式的付费端到端验收证据，不保证声纹克隆或内容一致。
```

## 9. 依据与维护边界

本 SOP 的依据是：

- `backend/internal/server/routes/gateway.go`：独立路由；
- `backend/internal/handler/minimax_h3.go`：权限、HTTP 返回、下载条件；
- `backend/internal/service/minimax_h3.go`：参数校验、原始字节幂等、状态机、费用快照和结算；
- `backend/internal/service/minimax_h3_request.go`：原生 task/conditions 校验、素材大小、MIME、数量、模式推断；
- 仓库 `docs/minimax-h3.md`：参考请求示例与供应商说明，协议参考为 <https://www.getoken.tech/dashboard/tokens> 的 H3 示例；
- 已完成的 5 秒 / 768p 文生视频端到端调用：两次下载相同 SHA256，实际 1344×768，单次扣费。

这不是 MiniMax 官方通用 API 文档，而是本 Sub2API fork 的独立 H3 接口调用规范。部署版本、域名、分组、供应商或价格变化后需重新核对；网关接受的参数不等于所有上游都支持。不要将其他规格、图生视频/混合参考/音色参考效果、内容一致性、永久可下载或所有账务场景宣称为已验证。

## 10. 网页端「视频任务」页

登录用户可在控制台的视频任务页查看自己的 H3 任务。该页使用登录态（JWT），不是 API Key，接口如下，供前端/排障参考：

| 操作 | 方法与路径 | 说明 |
| --- | --- | --- |
| 任务列表 | `GET /api/v1/user/video-tasks?page=&page_size=&status=` | 仅返回本人任务；`page_size` 最大 100；`status` 按生成状态过滤 |
| 下载成片 | `GET /api/v1/user/video-tasks/{id}/content` | 仅限本人、且 `completed + verified + settled`；否则 409 并带 `Retry-After: 20` |
| 立即刷新 | `POST /api/v1/user/video-tasks/{id}/refresh` | 立即推进一次该任务的上游轮询；不会重新提交生成 |

列表中的费用列：`settled` 为实际扣费，`pending` 为预估，其余为未扣费。刷新只推进已有任务，不能替代失败任务的重新生成；重新生成是新的付费业务请求。

## 11. 自部署 H3 上游（管理员）

网关也可以把 MiniMax 账户的 `base_url` 指向自部署的 H3 服务。调用方接口和本 SOP 完全相同，差别只在管理端：

- 账户 `base_url` 必须是 `https://` 地址并包含 `/v1` 前缀（例如 `https://h3.example.com/v1`）；网关在其后拼接 `/videos/generations`、`/videos/{id}`、`/videos/{id}/content`。
- 自部署服务需实现 `POST {base_url}/videos/generations`（返回任务 `id`）、`GET {base_url}/videos/{id}`（`status` 为 `completed`/`succeeded`/`done` 视为成功；`failed`/`cancelled`/`canceled`/`expired` 视为失败且不计费；其他值继续轮询）和 `GET {base_url}/videos/{id}/content`（返回 MP4），统一 `Authorization: Bearer <账户 Key>` 鉴权。
- 网关的 4–15 秒、四档分辨率只是网关侧校验；自部署服务支持的时长、分辨率、参考模式以服务自身为准，超出时会在提交阶段被拒，任务直接变为 `failed + not_billable`。对外开放前应把分组可用规格和自部署能力对齐，或告知调用方只用已支持的组合。
- 提交被拒的原因会写入网关日志（H3 提交失败日志带上游响应摘要），排障时按任务 ID 和提交时间查日志。
- 直连自测（不经网关、不计费）：在能访问自部署服务的机器上，用账户 Key 按上面三个接口提交一单 5 秒 768p `t2va`。一次实测结果：提交 0.2 秒返回 202，约 51 秒完成，下载到 1344×768、24fps、5.0 秒、带 AAC 音轨的 MP4。直连成功只证明上游可用，网关派单、交付校验和计费仍需用调用方 Key 走一次本 SOP 的完整流程确认。

-- Update the user-facing Seedance video guide content.
--
-- Idempotent: updates the existing item with id "seedance_video_guide" when present;
-- otherwise inserts it immediately after the API key configuration guide when found.

DO $$
DECLARE
    v_raw            text;
    v_items          jsonb;
    v_next           jsonb := '[]'::jsonb;
    v_elem           jsonb;
    v_new_item       jsonb;
    v_inserted       boolean := false;
    v_found_existing boolean := false;
    v_sort_order     int := 0;
    v_content        text := $md$
# Seedance 视频调用说明

Base URL:

```text
https://cc-ai.xyz/v1
```

认证方式:

```http
Authorization: Bearer 你的key
Content-Type: application/json
```

视频生成是异步任务。先提交任务拿到 `task_id`，再轮询任务状态，完成后从响应中的 `video_url`、`result_url` 或嵌套结果字段里读取视频链接。

## Seedance

Seedance 2.0 全能视频生成(即梦 / Sora 体系)-- 文生 / 图生 / 多图组合 / 首帧·首尾帧 / 参考视频 / 参考音频，一个接口全包。异步:提交拿 `task_id`，轮询到 `SUCCESS` 取视频。走 `/v1/video/generations`(不是 `/v1/chat/completions`，后者 404)。

需先在「API 密钥」页创建一把「seedance逆向低价」档的 key(创建密钥时在档次里选它)。该 key 专用于下列 `seedance-2.0-720` / `seedance-2.0-1080` 模型；调别的模型请用默认档 key。

### 模型与价格(按视频秒数)

| 模型 | 分辨率 | 原价 ¥/秒 | 10 秒 / 15 秒原价 |
| --- | --- | --- | --- |
| `seedance-2.0-720` | 720P | ¥1.00 | ¥10.00 / ¥15.00 |
| `seedance-2.0-1080` | 1080P | ¥1.20 | ¥12.00 / ¥18.00 |

按视频秒数计费；`seconds` 控制时长，当前支持 `10` / `15`(字符串)。分辨率由模型名决定。

### 1) 提交任务(文生视频)

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer 你的key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-720",
    "prompt": "霓虹雨夜街头的电影感跟拍镜头，缓慢推进，35mm 颗粒",
    "aspect_ratio": "16:9",
    "seconds": "10"
  }'
# -> { "task_id": "task_xxx", "object": "video", "status": "queued" }
```

### 2) 轮询直到完成

```bash
curl https://cc-ai.xyz/v1/video/generations/task_xxx \
  -H "Authorization: Bearer 你的key"
# status: in_progress ... 几分钟后 "status": "completed"
# 视频直链在响应的 video_url 字段(公网 .mp4)
```

务必轮询到 `status` 变 `completed` / `SUCCESS` 再取视频。生成中(`in_progress`)时 `video_url` 为空(或临时链)，取了也打不开，这是「扣钱没出片」最常见的原因。完成后 `video_url` 是公网直链，可直接播放 / 下载。

### 参数总表

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `model` | 必填 | `seedance-2.0-720`(720P) / `seedance-2.0-1080`(1080P) |
| `prompt` | 必填 | 画面提示词；多素材时用 `@Image1` / `@Video1` / `@Audio1` 显式指代 |
| `aspect_ratio` | 否 | `16:9`(默认) / `9:16` / `1:1` / `4:3` / `3:4` / `21:9` |
| `seconds` | 否 | 时长(字符串)，`"10"` / `"15"`，默认 10 |
| `image_url` | 否 | 单张参考图(URL 或 base64 dataURL) |
| `reference_image_urls` | 否 | 多张参考图数组(<=9)，`@ImageN` 对应第 N 张 |
| `reference_videos` | 否 | 参考视频数组(<=3，总时长 <=15s) |
| `audio_url` / `reference_audios` | 否 | 参考音频(<=3，mp3/wav/m4a 等)，需同时带 >=1 张参考图 |
| `video_config.reference_mode` | 否 | `auto`(默认，多图参考) / `start_frame`(正好 1 图=首帧) / `start_end`(正好 2 图=首尾帧) |

### @ 引用语法(多素材必读)

多素材组合时，模型靠 `prompt` 里的 `@` 标记识别每个素材的角色：`@Image1` = `reference_image_urls` 第 1 张、`@Video1` = `reference_videos` 第 1 个、`@Audio1` = `reference_audios` 第 1 个，依此类推。不显式 `@` 指代，模型会瞎猜哪张图是什么。

### 玩法示例

#### 1) 图生视频(单图)

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer 你的key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-720",
    "prompt": "@Image1 的人物开始走路，镜头跟随推进",
    "seconds": "10",
    "image_url": "https://你的图床/start.jpg"
  }'
```

#### 2) 多图组合(角色 + 场景，@ 引用)

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer 你的key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-1080",
    "prompt": "@Image1 的角色，在 @Image2 的场景里跳舞，宽银幕镜头",
    "aspect_ratio": "21:9",
    "seconds": "15",
    "reference_image_urls": [
      "https://你的图床/role.jpg",
      "https://你的图床/scene.jpg"
    ]
  }'
```

#### 3) 首帧(start_frame，正好 1 张图)

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer 你的key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-720",
    "prompt": "从这个画面开始，镜头缓慢推进，人物转身",
    "seconds": "10",
    "image_url": "https://你的图床/start.jpg",
    "video_config": {
      "reference_mode": "start_frame"
    }
  }'
```

#### 4) 首尾帧(start_end，正好 2 张图)

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer 你的key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-720",
    "prompt": "从第一张画面平滑过渡到第二张，自然运镜",
    "seconds": "10",
    "reference_image_urls": [
      "https://你的图床/first.jpg",
      "https://你的图床/last.jpg"
    ],
    "video_config": {
      "reference_mode": "start_end"
    }
  }'
```

#### 5) 全能参考(图 + 视频 + 音频，卡点 / 配乐)

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer 你的key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-720",
    "prompt": "@Image1 角色随 @Audio1 的节奏起舞，运镜参考 @Video1",
    "seconds": "15",
    "reference_image_urls": [
      "https://你的图床/role.jpg"
    ],
    "reference_videos": [
      "https://你的视频/camera.mp4"
    ],
    "audio_url": "https://你的音频/track.mp3"
  }'
```

用音频(`audio_url` / `reference_audios`)时必须同时带 >=1 张参考图，否则上游报错。

### Python 完整示例(提交 + 轮询)

```python
import time
import requests

BASE = "https://cc-ai.xyz/v1/video/generations"
H = {"Authorization": "Bearer 你的key", "Content-Type": "application/json"}

task = requests.post(BASE, headers=H, json={
    "model": "seedance-2.0-720",
    "prompt": "一只橘猫在窗台上伸懒腰，慢镜头，暖色调",
    "aspect_ratio": "16:9",
    "seconds": "10",
}).json()

tid = task.get("task_id") or task.get("id")

def pick(d):
    out = None

    def w(n):
        nonlocal out
        if isinstance(n, dict):
            v = n.get("video_url")
            if isinstance(v, str) and v.startswith("http") and not out:
                out = v
            for x in n.values():
                w(x)
        elif isinstance(n, list):
            for x in n:
                w(x)

    w(d)
    return out

for _ in range(120):
    r = requests.get(f"{BASE}/{tid}", headers=H).json()
    st = str(r.get("data", {}).get("status") or r.get("status") or "").lower()
    if st in ("completed", "success", "failed", "failure"):
        print(st, "->", pick(r))
        break
    time.sleep(8)
```

### 常见问题

| 现象 | 原因 / 解决 |
| --- | --- |
| 无可用渠道 / 模型不存在 | key 不是「seedance逆向低价」档，或模型名拼错(只有 `seedance-2.0-720` / `seedance-2.0-1080`) |
| `seconds` 报错 / 不生效 | 必须是字符串，且只能 `"10"` / `"15"` |
| 多图但角色错乱 | `prompt` 里用 `@Image1` / `@Image2` 显式指代每张图 |
| 音频报错 | 用音频时必须同时带至少一张参考图 |
| 视频链接过段时间失效 | 临时直链，拿到尽快转存到自己存储 |
| 任务很久仍生成中 | 高峰排队正常，1080P 更慢；耐心轮询，别频繁重建 |
| 偶发 5xx | 上游波动，稍后重试 |

## 国际seedace

即梦 Seedance 2.0 官方满血源(质量优先，与上方普通 Seedance 是两套独立的源与价格)。支持文生 / 图生 / 首尾帧 / 参考音频，异步接口(提交 -> 轮询)，按视频秒数计费。

需先在「API 密钥」页创建一把「国际seedace」档的 key(创建密钥时在档次里选它)。该 key 专用于下列 `dreamina-seedance-2-0-*` 模型；调别的模型请用默认档 key。

视频默认带声音：Seedance 2.0 会为画面自动生成 AI 环境音 / 音效，默认开启且不额外收费。不想要声音时传 `"generate_audio": false`；想让画面跟随你指定的音频(唱歌 / 卡点)见下方「参考音频」玩法。注意：上方普通 Seedance 是另一套独立的源，是否有声以那套源为准，要稳定有声请用本节的 `dreamina-seedance-2-0-*`。

### 模型与价格(按视频秒数)

| 模型(文生 / 图生·首尾帧·音频用 -ref) | 分辨率 | 文生原价 ¥/秒 | 带图(-ref)原价 ¥/秒 |
| --- | --- | --- | --- |
| `dreamina-seedance-2-0-480p[-ref]` | 480P | ¥0.52 | ¥0.33 |
| `dreamina-seedance-2-0-720p[-ref]` | 720P | ¥1.12 | ¥0.68 |
| `dreamina-seedance-2-0-1080p[-ref]` | 1080P | ¥2.78 | ¥1.69 |
| `dreamina-seedance-2-0-fast-480p[-ref]` | 480P 快 | ¥0.42 | ¥0.24 |
| `dreamina-seedance-2-0-fast-720p[-ref]` | 720P 快 | ¥0.91 | ¥0.53 |

纯文字用不带 `-ref` 的；带图 / 首尾帧 / 音频用带 `-ref` 的。`duration` 控制秒数(默认 4)。

### 1) 文生视频

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的国际seedaceKEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dreamina-seedance-2-0-720p",
    "prompt": "一只橘猫在窗台伸懒腰，暖色调",
    "duration": 5
  }'
```

### 2) 图生 / 参考生(-ref + image)

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的国际seedaceKEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "镜头缓缓推进，画面动起来",
    "duration": 5,
    "image": "https://你的图床/photo.jpg"
  }'
# image 支持 http 链接或 base64 data URL；多图用 images:[...](<=9)；也兼容 image_url / reference_image_urls
```

### 3) 首尾帧过渡(-ref + first_frame/last_frame)

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的国际seedaceKEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "从第一张平滑过渡到第二张",
    "duration": 5,
    "first_frame": "https://你的图床/first.jpg",
    "last_frame": "https://你的图床/last.jpg"
  }'
```

### 4) 参考音频(-ref + image + audio_url)

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的国际seedaceKEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "这个人随节奏唱歌",
    "duration": 5,
    "image": "https://你的图床/singer.jpg",
    "audio_url": "https://你的音频/song.mp3"
  }'
# 用音频时必须同时带至少一张图
```

### 5) 参考视频(-ref + reference_videos)

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的国际seedaceKEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "运镜参考 @Video1，把场景换成雪天",
    "duration": 5,
    "reference_videos": [
      "https://你的视频/camera.mp4"
    ]
  }'
# reference_videos 数组(<=3)；可与参考图同用，prompt 里 @Video1 / @Image1 指代
```

### 轮询取片

```bash
curl https://cc-ai.xyz/v1/video/generations/task_xxx \
  -H "Authorization: Bearer sk-你的国际seedaceKEY"
# data.status=SUCCESS 后，视频在 data.data.video_url(或 result_url，等价)
```

参考图别太小(约 256px 以下会被上游拒，用 >=512px 稳)；视频直链是临时的，拿到尽快转存。首尾帧也可用 `video_config.reference_mode = start_frame/start_end` 指定。参考视频用 `reference_videos`(数组 <=3，单段建议 <=15s)，与图片同走转存，可与参考图 / 音频同用；参考视频分辨率需 >=480p(像素 >=409600，360p 等过小会被上游拒)。

### 参数总表

| 参数 | 适用 | 说明 |
| --- | --- | --- |
| `model` | 必填 | 上表模型名(决定分辨率 / 计费档) |
| `prompt` | 必填 | 画面描述 |
| `duration` | 否 | 秒数，默认 4(价格 = 每秒价 x 秒数)；同义字段 `seconds` |
| `aspect_ratio` | 否 | `16:9`(默认) / `9:16` / `1:1` / `4:3` / `3:4` / `21:9` |
| `image` / `images` | -ref | 参考图(单 / 多 <=9)；http 链接或 base64；同义字段 `image_url` / `reference_image_urls` |
| `first_frame` / `last_frame` | -ref | 首帧 / 尾帧图(首尾帧过渡) |
| `reference_videos` | -ref | 参考视频数组(<=3，单段建议 <=15s)；http 链接或 base64 |
| `audio_url` | -ref | 参考音频(直链或 base64)，需配 >=1 张图 |
| `generate_audio` | 全部 | 是否生成 AI 声音，默认 true(出声)；传 false 得静音视频。不额外收费 |

### Python 完整示例(提交 + 轮询)

```python
import time
import requests

BASE = "https://cc-ai.xyz/v1"
KEY = "你的国际seedace key"
H = {"Authorization": f"Bearer {KEY}", "Content-Type": "application/json"}

# 图生(参考图)；文生去掉 image、换不带 -ref 的模型即可
task = requests.post(f"{BASE}/video/generations", headers=H, json={
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "镜头缓缓推进，画面动起来",
    "duration": 5,
    "image": "https://你的图床/photo.jpg",
}).json()

tid = task.get("task_id") or task.get("id")

def pick(d):
    out = None

    def w(n):
        nonlocal out
        if isinstance(n, dict):
            v = n.get("video_url")
            if isinstance(v, str) and v.startswith("http") and not out:
                out = v
            for x in n.values():
                w(x)
        elif isinstance(n, list):
            for x in n:
                w(x)

    w(d)
    return out

for _ in range(120):
    r = requests.get(f"{BASE}/video/generations/{tid}", headers=H).json()
    st = str(r.get("data", {}).get("status") or r.get("status") or "").upper()
    if st in ("SUCCESS", "FAILURE", "FAILED"):
        print(st, "->", pick(r) or r.get("data", {}).get("result_url"))
        break
    time.sleep(8)
```

### 常见问题

| 现象 | 原因 / 解决 |
| --- | --- |
| 无可用渠道 / 模型不存在 | key 不是「国际seedace」档，或模型名拼错 |
| 视频没有声音 | 本节模型默认带声音；若用的是普通 `seedance-2.0`(另一套源)或传了 `generate_audio:false` 会静音，改用 `dreamina-seedance-2-0-*` 且别关音频 |
| 401 鉴权失败 | 检查 `Authorization: Bearer` 头与 key |
| 参考图报 Asset provider error | 图太小(<~256px)，换 >=512px |
| `-ref` 模型报 requires an image | `-ref` 必须带 `image` / `first_frame` / `last_frame` |
| 音频报 requires reference_image | 用音频时必须同时带至少一张图 |
| 视频链接过段时间失效 | 临时直链，拿到尽快转存到自己存储 |
| 任务很久仍生成中 | 高峰排队正常，1080P 更慢；耐心轮询，别频繁重建 |
| 偶发 5xx | 上游波动，稍后重试 |
$md$;
BEGIN
    SELECT value INTO v_raw
      FROM settings WHERE key = 'custom_menu_items';

    IF COALESCE(v_raw, '') = '' OR v_raw = 'null' THEN
        v_items := '[]'::jsonb;
    ELSE
        v_items := v_raw::jsonb;
    END IF;

    v_new_item := jsonb_build_object(
        'id',         'seedance_video_guide',
        'label',      'seedace视频调用说明',
        'icon_svg',   '<svg fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 6.75h9A2.25 2.25 0 0115.75 9v6A2.25 2.25 0 0113.5 17.25h-9A2.25 2.25 0 012.25 15V9A2.25 2.25 0 014.5 6.75z"/></svg>',
        'url',        'md:seedance-video-guide',
        'page_slug',  'seedance-video-guide',
        'content_md', v_content,
        'visibility', 'user',
        'sort_order', 0
    );

    FOR v_elem IN SELECT jsonb_array_elements(v_items) LOOP
        IF v_elem ->> 'id' = 'seedance_video_guide' THEN
            v_next := v_next || jsonb_build_array(
                v_new_item || jsonb_build_object(
                    'sort_order',
                    CASE
                        WHEN COALESCE(v_elem ->> 'sort_order', '') ~ '^[0-9]+$'
                        THEN (v_elem ->> 'sort_order')::int
                        ELSE v_sort_order
                    END
                )
            );
            v_found_existing := true;
        ELSE
            v_next := v_next || jsonb_build_array(
                jsonb_set(v_elem, '{sort_order}', to_jsonb(v_sort_order), true)
            );

            IF NOT v_inserted
               AND NOT v_found_existing
               AND (
                 v_elem ->> 'id' IN ('api_key_config_guide', 'api-key-config-guide', 'api_key_guide')
                 OR v_elem ->> 'label' IN ('API秘钥配置说明', 'API密钥配置说明', 'API 密钥配置说明', 'API Key 配置说明')
               ) THEN
                v_sort_order := v_sort_order + 1;
                v_next := v_next || jsonb_build_array(
                    v_new_item || jsonb_build_object('sort_order', v_sort_order)
                );
                v_inserted := true;
            END IF;
        END IF;
        v_sort_order := v_sort_order + 1;
    END LOOP;

    IF NOT v_found_existing AND NOT v_inserted THEN
        v_next := v_next || jsonb_build_array(
            v_new_item || jsonb_build_object('sort_order', v_sort_order)
        );
    END IF;

    INSERT INTO settings (key, value)
    VALUES ('custom_menu_items', v_next::text)
    ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;
END $$;

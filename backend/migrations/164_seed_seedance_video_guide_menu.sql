-- Seeds the user-facing Seedance video guide as a configurable Markdown custom page.
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
Authorization: Bearer sk-你的CCAI密钥
Content-Type: application/json
```

视频生成是异步任务。先提交任务拿到 `task_id`，再轮询任务状态，完成后从响应中的 `video_url`、`result_url` 或嵌套结果字段里读取视频链接。

## Seedance

适用于普通 Seedance 视频生成。接口固定使用 `/v1/video/generations`，不要使用聊天接口。

**可用模型**

| 模型 | 分辨率 | 说明 |
| --- | --- | --- |
| `seedance-2.0-720` | 720P | 文生视频、图生视频、首尾帧、参考视频、参考音频 |
| `seedance-2.0-1080` | 1080P | 文生视频、图生视频、首尾帧、参考视频、参考音频 |

**提交任务：文生视频**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-720",
    "prompt": "霓虹雨夜街头的电影感跟拍镜头，缓慢推进，35mm 颗粒",
    "aspect_ratio": "16:9",
    "seconds": "10"
  }'
```

**轮询任务**

```bash
curl https://cc-ai.xyz/v1/video/generations/task_xxx \
  -H "Authorization: Bearer sk-你的CCAI密钥"
```

任务状态为 `completed`、`SUCCESS` 或同义成功状态时，读取返回体里的视频链接。高峰期任务会排队，轮询间隔保持在 5 到 10 秒。

**图生视频**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-720",
    "prompt": "@Image1 的人物开始走路，镜头跟随推进",
    "seconds": "10",
    "image_url": "https://your-domain.example/start.jpg"
  }'
```

**多图参考**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-1080",
    "prompt": "@Image1 的角色在 @Image2 的场景里跳舞，宽银幕镜头",
    "aspect_ratio": "21:9",
    "seconds": "15",
    "reference_image_urls": [
      "https://your-domain.example/role.jpg",
      "https://your-domain.example/scene.jpg"
    ]
  }'
```

**首帧**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-720",
    "prompt": "从这个画面开始，镜头缓慢推进，人物转身",
    "seconds": "10",
    "image_url": "https://your-domain.example/start.jpg",
    "video_config": {
      "reference_mode": "start_frame"
    }
  }'
```

**首尾帧**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-720",
    "prompt": "从第一张画面平滑过渡到第二张，自然运镜",
    "seconds": "10",
    "reference_image_urls": [
      "https://your-domain.example/first.jpg",
      "https://your-domain.example/last.jpg"
    ],
    "video_config": {
      "reference_mode": "start_end"
    }
  }'
```

**参考视频与参考音频**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "seedance-2.0-720",
    "prompt": "@Image1 角色随 @Audio1 的节奏起舞，运镜参考 @Video1",
    "seconds": "15",
    "reference_image_urls": [
      "https://your-domain.example/role.jpg"
    ],
    "reference_videos": [
      "https://your-domain.example/camera.mp4"
    ],
    "audio_url": "https://your-domain.example/music.mp3"
  }'
```

**Python 完整示例**

```python
import time
import requests

BASE = "https://cc-ai.xyz/v1/video/generations"
HEADERS = {
    "Authorization": "Bearer sk-你的CCAI密钥",
    "Content-Type": "application/json",
}

task = requests.post(BASE, headers=HEADERS, json={
    "model": "seedance-2.0-720",
    "prompt": "一只橘猫在窗台上伸懒腰，慢镜头，暖色调",
    "aspect_ratio": "16:9",
    "seconds": "10",
}).json()

task_id = task.get("task_id") or task.get("id")

def pick_video_url(node):
    if isinstance(node, dict):
        for key in ("video_url", "result_url", "url"):
            value = node.get(key)
            if isinstance(value, str) and value.startswith("http"):
                return value
        for value in node.values():
            found = pick_video_url(value)
            if found:
                return found
    if isinstance(node, list):
        for item in node:
            found = pick_video_url(item)
            if found:
                return found
    return None

for _ in range(120):
    result = requests.get(f"{BASE}/{task_id}", headers=HEADERS).json()
    status = str(result.get("status") or result.get("data", {}).get("status") or "").upper()
    if status in ("COMPLETED", "SUCCESS", "FAILED", "FAILURE"):
        print(status, pick_video_url(result))
        break
    time.sleep(8)
```

## 海外 Seedance

适用于海外 Seedance 高质量视频生成。该接口同样是异步任务，支持文生、图生、首尾帧、参考视频和参考音频。

**可用模型**

| 模型 | 分辨率 | 说明 |
| --- | --- | --- |
| `dreamina-seedance-2-0-480p` | 480P | 文生视频 |
| `dreamina-seedance-2-0-480p-ref` | 480P | 图生、首尾帧、参考视频、参考音频 |
| `dreamina-seedance-2-0-720p` | 720P | 文生视频 |
| `dreamina-seedance-2-0-720p-ref` | 720P | 图生、首尾帧、参考视频、参考音频 |
| `dreamina-seedance-2-0-1080p` | 1080P | 文生视频 |
| `dreamina-seedance-2-0-1080p-ref` | 1080P | 图生、首尾帧、参考视频、参考音频 |
| `dreamina-seedance-2-0-fast-480p` | 480P | 快速文生视频 |
| `dreamina-seedance-2-0-fast-480p-ref` | 480P | 快速图生和参考素材生成 |

`-ref` 模型用于带参考素材的生成方式，例如图生视频、首尾帧、参考视频和参考音频。

**提交任务：文生视频**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dreamina-seedance-2-0-720p",
    "prompt": "清晨的城市街道，行人和车辆自然移动，电影感运镜",
    "duration": 5,
    "aspect_ratio": "16:9",
    "generate_audio": true
  }'
```

**轮询任务**

```bash
curl https://cc-ai.xyz/v1/video/generations/task_xxx \
  -H "Authorization: Bearer sk-你的CCAI密钥"
```

**图生视频**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "镜头缓缓推进，画面中的人物转身看向远处",
    "duration": 5,
    "image": "https://your-domain.example/photo.jpg"
  }'
```

**首尾帧过渡**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "从第一张画面自然过渡到第二张画面，保持主体一致",
    "duration": 5,
    "first_frame": "https://your-domain.example/first.jpg",
    "last_frame": "https://your-domain.example/last.jpg"
  }'
```

**参考视频**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "参考 @Video1 的镜头节奏，生成一段户外运动视频",
    "duration": 5,
    "reference_videos": [
      "https://your-domain.example/camera-reference.mp4"
    ]
  }'
```

**参考音频**

```bash
curl https://cc-ai.xyz/v1/video/generations \
  -H "Authorization: Bearer sk-你的CCAI密钥" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "@Image1 中的人物跟随 @Audio1 的节奏跳舞",
    "duration": 5,
    "image": "https://your-domain.example/person.jpg",
    "audio_url": "https://your-domain.example/music.mp3"
  }'
```

**Python 完整示例**

```python
import time
import requests

BASE = "https://cc-ai.xyz/v1"
KEY = "sk-你的CCAI密钥"
HEADERS = {
    "Authorization": f"Bearer {KEY}",
    "Content-Type": "application/json",
}

task = requests.post(f"{BASE}/video/generations", headers=HEADERS, json={
    "model": "dreamina-seedance-2-0-720p-ref",
    "prompt": "镜头缓缓推进，画面动起来",
    "duration": 5,
    "image": "https://your-domain.example/photo.jpg",
}).json()

task_id = task.get("task_id") or task.get("id")

def pick_video_url(node):
    if isinstance(node, dict):
        for key in ("video_url", "result_url", "url"):
            value = node.get(key)
            if isinstance(value, str) and value.startswith("http"):
                return value
        for value in node.values():
            found = pick_video_url(value)
            if found:
                return found
    if isinstance(node, list):
        for item in node:
            found = pick_video_url(item)
            if found:
                return found
    return None

for _ in range(120):
    result = requests.get(f"{BASE}/video/generations/{task_id}", headers=HEADERS).json()
    status = str(result.get("data", {}).get("status") or result.get("status") or "").upper()
    if status in ("SUCCESS", "COMPLETED", "FAILED", "FAILURE"):
        print(status, pick_video_url(result))
        break
    time.sleep(8)
```

**常见问题**

| 现象 | 处理方式 |
| --- | --- |
| 401 鉴权失败 | 检查 `Authorization: Bearer` 请求头和 CCAI API 密钥。 |
| 模型不存在 | 检查密钥分组是否支持该模型，以及模型名是否拼写正确。 |
| `-ref` 模型要求图片 | 使用 `-ref` 模型时提交 `image`、`first_frame` 或 `last_frame`。 |
| 参考音频失败 | 参考音频通常需要同时提供至少一张参考图。 |
| 视频链接失效 | 生成结果链接可能有有效期，业务需要长期保存时应及时转存。 |
| 任务长时间生成中 | 视频生成会排队，1080P 更慢，保持轮询，不要频繁重建任务。 |
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

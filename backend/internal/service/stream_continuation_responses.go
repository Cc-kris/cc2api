package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ---------------------------------------------------------------------------
// OpenAI Responses (/v1/responses SSE) 续写
//
// 首次尝试：只记录客户端已看到的状态（response.id、sequence_number、output_index、
// 当前未结束的 message 输出项及其已输出正文）。
// 续写尝试：丢弃重复的 response.created / response.in_progress，
// 把新账号的第一个 message 输出项并入旧的未结束 message 项（复用 item_id / output_index），
// 其余输出项的 output_index 顺延，sequence_number 保持单调递增，response.id 保持不变。
// ---------------------------------------------------------------------------

// responsesContinuationInstruction 续写时附加给新账号的指令（Responses 没有严格意义的 assistant 预填）。
const responsesContinuationInstruction = "Your previous reply above was interrupted mid-output. Continue exactly from where it stopped. " +
	"Do not repeat any text that was already written, do not add any preamble or acknowledgement, output only the remaining part."

type responsesOpenMessage struct {
	itemID       string
	outputIndex  int
	contentIndex int
	text         strings.Builder
}

type responsesContState struct {
	responseID      string
	lastSeq         int64
	nextOutputIndex int
	openMsg         *responsesOpenMessage

	// 当前续写尝试
	outputMap      map[int]int
	merged         bool
	mergedUpItemID string
	partMerged     bool

	// 被合并的 message 项结束后保留的最终正文与 id（用于修正终止事件里的 response.output）
	lastMergedText   string
	lastMergedItemID string
}

func (sc *StreamContinuation) responses() *responsesContState {
	if sc.resp == nil {
		sc.resp = &responsesContState{}
	}
	return sc.resp
}

func (sc *StreamContinuation) beginResponsesAttempt() {
	if sc.resp == nil {
		return
	}
	if !sc.wroteContent {
		// 上一次尝试的内容并未真正推给客户端：重新开始记录
		sc.resp = &responsesContState{}
		return
	}
	sc.resp.outputMap = make(map[int]int)
	sc.resp.merged = false
	sc.resp.mergedUpItemID = ""
	sc.resp.partMerged = false
}

// BuildResponsesContinuationBody 在已有输出时，把已输出正文作为 assistant 消息追加到 input，
// 再追加一条续写指令，让新账号从断点接着写。
func (sc *StreamContinuation) BuildResponsesContinuationBody(body []byte) []byte {
	if !sc.Active() {
		return body
	}
	text := sc.text.String()
	trimmed := strings.TrimRight(text, " \t\r\n")
	sc.pendingStripPrefix = text[len(trimmed):]
	if trimmed == "" {
		return body
	}
	out := body
	input := gjson.GetBytes(out, "input")
	if input.Type == gjson.String {
		v, err := sjson.SetBytes(out, "input", []map[string]any{{
			"type":    "message",
			"role":    "user",
			"content": []map[string]any{{"type": "input_text", "text": input.String()}},
		}})
		if err != nil {
			return body
		}
		out = v
	} else if !input.Exists() {
		v, err := sjson.SetRawBytes(out, "input", []byte("[]"))
		if err != nil {
			return body
		}
		out = v
	} else if !input.IsArray() {
		return body
	}
	v, err := sjson.SetBytes(out, "input.-1", map[string]any{
		"type":    "message",
		"role":    "assistant",
		"content": []map[string]any{{"type": "output_text", "text": trimmed}},
	})
	if err != nil {
		return body
	}
	v, err = sjson.SetBytes(v, "input.-1", map[string]any{
		"type":    "message",
		"role":    "developer",
		"content": []map[string]any{{"type": "input_text", "text": responsesContinuationInstruction}},
	})
	if err != nil {
		return body
	}
	return v
}

// RewriteResponsesData 处理一个即将写给客户端的 Responses SSE data（JSON）。
// 返回 keep=false 表示该事件不写给客户端。startsOutput 表示该事件属于正文输出（非 preamble）。
func (sc *StreamContinuation) RewriteResponsesData(data string, startsOutput bool) (string, bool) {
	if sc == nil {
		return data, true
	}
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" || !gjson.Valid(trimmed) {
		return data, true
	}
	st := sc.responses()
	evType := gjson.Get(trimmed, "type").String()

	if !sc.continuing {
		sc.trackResponses(st, evType, trimmed)
		if startsOutput {
			sc.wroteContent = true
		}
		return data, true
	}

	switch evType {
	case "response.created", "response.in_progress":
		return "", false
	}

	out := trimmed
	switch evType {
	case "response.output_item.added":
		upIdx := int(gjson.Get(out, "output_index").Int())
		itemType := gjson.Get(out, "item.type").String()
		if itemType == "message" && st.openMsg != nil && !st.merged {
			st.merged = true
			st.mergedUpItemID = gjson.Get(out, "item.id").String()
			st.outputMap[upIdx] = st.openMsg.outputIndex
			return "", false
		}
		newIdx := st.nextOutputIndex
		st.nextOutputIndex++
		st.outputMap[upIdx] = newIdx
		out = setJSONInt(out, "output_index", newIdx)
		if isResponsesToolItemType(itemType) {
			sc.sawToolUse = true
		}
		if itemType == "message" && st.openMsg == nil {
			st.openMsg = &responsesOpenMessage{itemID: gjson.Get(out, "item.id").String(), outputIndex: newIdx}
		}
	case "response.content_part.added":
		if st.merged && gjson.Get(out, "item_id").String() == st.mergedUpItemID && !st.partMerged {
			st.partMerged = true
			return "", false
		}
		out = sc.remapResponsesItem(st, out)
	case "response.output_text.delta":
		isMerged := st.merged && gjson.Get(out, "item_id").String() == st.mergedUpItemID
		out = sc.remapResponsesItem(st, out)
		delta := gjson.Get(out, "delta").String()
		if isMerged && !sc.firstDeltaDone && delta != "" {
			sc.firstDeltaDone = true
			if p := sc.pendingStripPrefix; p != "" {
				if strings.HasPrefix(delta, p) {
					delta = delta[len(p):]
				} else {
					delta = strings.TrimLeft(delta, " \t\r\n")
				}
				if v, err := sjson.Set(out, "delta", delta); err == nil {
					out = v
				}
			}
		}
		sc.text.WriteString(delta)
		if st.openMsg != nil && gjson.Get(out, "item_id").String() == st.openMsg.itemID {
			st.openMsg.text.WriteString(delta)
		}
	case "response.output_text.done":
		isMerged := st.merged && gjson.Get(out, "item_id").String() == st.mergedUpItemID
		out = sc.remapResponsesItem(st, out)
		if isMerged && st.openMsg != nil {
			out = setJSONString(out, "text", st.openMsg.text.String())
		}
	case "response.content_part.done":
		isMerged := st.merged && gjson.Get(out, "item_id").String() == st.mergedUpItemID
		out = sc.remapResponsesItem(st, out)
		if isMerged && st.openMsg != nil && gjson.Get(out, "part.type").String() == "output_text" {
			out = setJSONString(out, "part.text", st.openMsg.text.String())
		}
	case "response.output_item.done":
		isMerged := st.merged && gjson.Get(out, "item.id").String() == st.mergedUpItemID
		upIdx := int(gjson.Get(out, "output_index").Int())
		if idx, ok := st.outputMap[upIdx]; ok {
			out = setJSONInt(out, "output_index", idx)
		}
		if isMerged && st.openMsg != nil {
			out = setJSONString(out, "item.id", st.openMsg.itemID)
			out = patchResponsesMessageText(out, "item.content", st.openMsg.text.String())
			st.lastMergedText = st.openMsg.text.String()
			st.lastMergedItemID = st.openMsg.itemID
		}
		if st.openMsg != nil && (isMerged || gjson.Get(out, "item.id").String() == st.openMsg.itemID) {
			st.openMsg = nil
		}
	case "response.completed", "response.incomplete", "response.failed":
		if evType != "response.failed" {
			sc.sawStopReason = true
		}
		if st.mergedUpItemID != "" {
			out = sc.patchResponsesFinalOutput(st, out)
		}
	default:
		out = sc.remapResponsesItem(st, out)
		if strings.HasPrefix(evType, "response.function_call") || strings.HasPrefix(evType, "response.custom_tool_call") {
			sc.sawToolUse = true
		}
	}

	if st.responseID != "" && gjson.Get(out, "response.id").Exists() {
		out = setJSONString(out, "response.id", st.responseID)
	}
	if gjson.Get(out, "sequence_number").Exists() {
		st.lastSeq++
		out = setJSONInt(out, "sequence_number", int(st.lastSeq))
	}
	sc.wroteContent = true
	return out, true
}

func (sc *StreamContinuation) trackResponses(st *responsesContState, evType, data string) {
	if seq := gjson.Get(data, "sequence_number"); seq.Exists() && seq.Int() > st.lastSeq {
		st.lastSeq = seq.Int()
	}
	switch evType {
	case "response.created", "response.in_progress":
		if id := gjson.Get(data, "response.id").String(); id != "" {
			st.responseID = id
		}
	case "response.output_item.added":
		idx := int(gjson.Get(data, "output_index").Int())
		if idx+1 > st.nextOutputIndex {
			st.nextOutputIndex = idx + 1
		}
		itemType := gjson.Get(data, "item.type").String()
		if itemType == "message" {
			st.openMsg = &responsesOpenMessage{itemID: gjson.Get(data, "item.id").String(), outputIndex: idx}
		}
		if isResponsesToolItemType(itemType) {
			sc.sawToolUse = true
		}
	case "response.content_part.added":
		if st.openMsg != nil && gjson.Get(data, "item_id").String() == st.openMsg.itemID &&
			gjson.Get(data, "part.type").String() == "output_text" {
			st.openMsg.contentIndex = int(gjson.Get(data, "content_index").Int())
		}
	case "response.output_text.delta":
		delta := gjson.Get(data, "delta").String()
		sc.text.WriteString(delta)
		if st.openMsg != nil && gjson.Get(data, "item_id").String() == st.openMsg.itemID {
			st.openMsg.text.WriteString(delta)
		}
	case "response.output_item.done":
		if st.openMsg != nil && gjson.Get(data, "item.id").String() == st.openMsg.itemID {
			st.openMsg = nil
		}
	case "response.completed", "response.incomplete":
		sc.sawStopReason = true
	default:
		if strings.HasPrefix(evType, "response.function_call") || strings.HasPrefix(evType, "response.custom_tool_call") {
			sc.sawToolUse = true
		}
	}
}

// remapResponsesItem 把续写尝试中的 item_id / output_index / content_index 映射到客户端已看到的值。
func (sc *StreamContinuation) remapResponsesItem(st *responsesContState, data string) string {
	out := data
	if st.merged && st.openMsg != nil && gjson.Get(out, "item_id").String() == st.mergedUpItemID {
		out = setJSONString(out, "item_id", st.openMsg.itemID)
		out = setJSONInt(out, "output_index", st.openMsg.outputIndex)
		if gjson.Get(out, "content_index").Exists() {
			out = setJSONInt(out, "content_index", st.openMsg.contentIndex)
		}
		return out
	}
	if idxRes := gjson.Get(out, "output_index"); idxRes.Exists() {
		if idx, ok := st.outputMap[int(idxRes.Int())]; ok {
			out = setJSONInt(out, "output_index", idx)
		}
	}
	return out
}

// patchResponsesFinalOutput 修正终止事件里 response.output 中被合并的 message 项（id 与完整正文）。
func (sc *StreamContinuation) patchResponsesFinalOutput(st *responsesContState, data string) string {
	output := gjson.Get(data, "response.output")
	if !output.IsArray() {
		return data
	}
	out := data
	for i, item := range output.Array() {
		if item.Get("id").String() != st.mergedUpItemID {
			continue
		}
		base := "response.output." + itoa(i)
		if st.mergedFinalText() != "" {
			out = patchResponsesMessageText(out, base+".content", st.mergedFinalText())
		}
		if st.mergedItemID() != "" {
			out = setJSONString(out, base+".id", st.mergedItemID())
		}
	}
	return out
}

func (st *responsesContState) mergedFinalText() string {
	if st.openMsg != nil {
		return st.openMsg.text.String()
	}
	return st.lastMergedText
}

func (st *responsesContState) mergedItemID() string {
	if st.openMsg != nil {
		return st.openMsg.itemID
	}
	return st.lastMergedItemID
}

// patchResponsesMessageText 把 content 数组里第一个 output_text 的 text 替换为完整正文。
func patchResponsesMessageText(data, contentPath, text string) string {
	content := gjson.Get(data, contentPath)
	if !content.IsArray() {
		return data
	}
	for i, part := range content.Array() {
		if part.Get("type").String() == "output_text" {
			return setJSONString(data, contentPath+"."+itoa(i)+".text", text)
		}
	}
	return data
}

func isResponsesToolItemType(itemType string) bool {
	switch itemType {
	case "function_call", "custom_tool_call", "local_shell_call", "computer_call",
		"mcp_call", "code_interpreter_call", "image_generation_call", "apply_patch_call", "shell_call":
		return true
	}
	return false
}

func setJSONString(data, path, v string) string {
	if out, err := sjson.Set(data, path, v); err == nil {
		return out
	}
	return data
}

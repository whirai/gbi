package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
)

// 无状态 fake Anthropic Messages 端点:
// 按请求 messages 中 tool_result 块的数量决定步骤:
// 0 → tool_use get_schema;1 → tool_use execute_sql;2 → tool_use update_page;≥3 → 文本 end_turn。
// update_page 的 subjects 从 system prompt 的 [[GBI_CONTEXT]] 块解析。

var ctxRe = regexp.MustCompile(`\[\[GBI_CONTEXT\]\](.*?)\[\[/GBI_CONTEXT\]\]`)
var codeRe = regexp.MustCompile(`"code":"([^"]+)"`)

func main() {
	http.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string            `json:"role"`
				Content []json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		// system 可能是字符串或数组,单独由 extractSystemText 解析
		_ = json.Unmarshal(body, &req)

		toolResults := 0
		for _, m := range req.Messages {
			for _, c := range m.Content {
				var blk struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal(c, &blk)
				if blk.Type == "tool_result" {
					toolResults++
				}
			}
		}

		// 解析数据集上下文(system 可能是 []TextBlock 数组,拼成字符串再匹配)
		subjectID := "0"
		tableName := ""
		subjectCode := ""
		sysText := extractSystemText(body)
		if m := ctxRe.FindStringSubmatch(sysText); len(m) > 1 {
			var subs []struct {
				ID     int    `json:"id"`
				Code   string `json:"code"`
				Tables []struct {
					Name string `json:"name"`
				} `json:"tables"`
			}
			if err := json.Unmarshal([]byte(m[1]), &subs); err == nil && len(subs) > 0 {
				subjectID = fmt.Sprintf("%d", subs[0].ID)
				subjectCode = subs[0].Code
				if len(subs[0].Tables) > 0 {
					tableName = subs[0].Tables[0].Name
				}
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")

		// 渲染校验闭环分支:上一轮自动核验注入了 [渲染校验结果] 且含 labelTransform 报错时,
		// 发出修正版 update_page(图表1标题加"已修正"、移除坏标签),其余两图与坏版一致。
		if subjectCode == "cvms_e2e" && strings.Contains(lastUserText(req.Messages), "labelTransform") {
			writeToolUse(w, "toolu_fix", "update_page", fmt.Sprintf(
				`{"charts":[
					{"id":"top-brands","title":"车辆类别销量占比-已修正","width":"full","type":"interval",
					 "sql":"SELECT Brand, qty, ROUND(qty*100.0/SUM(qty) OVER(),2) AS pct FROM (SELECT Brand, SUM(Quantity) AS qty FROM %s GROUP BY Brand) ORDER BY qty DESC LIMIT 10",
					 "subjects":[%s],
					 "spec":{"type":"interval","encode":{"x":"Brand","y":"qty","color":"Brand"},"labels":[{"text":"pct","position":"top"}],"axis":{"x":{"labelAutoRotate":true}}}},
					{"id":"mass-category","title":"吨位段×车辆类别结构","width":"1/2","type":"interval",
					 "sql":"SELECT CASE WHEN GrossMass>=25000 THEN '重型' WHEN GrossMass>=12000 THEN '中型' ELSE '轻/微型' END AS seg, VehicleCategory, SUM(Quantity) AS total FROM %s GROUP BY seg, VehicleCategory ORDER BY total DESC LIMIT 50",
					 "subjects":[%s],
					 "spec":{"type":"interval","encode":{"x":"seg","y":"total","color":"VehicleCategory"},"transform":[{"type":"stackY"}],"legend":{"color":{"position":"top"}}}},
					{"id":"top-cities","title":"Top10 城市销量排名","width":"1/2","type":"interval",
					 "sql":"SELECT Province, City, total, RANK() OVER(ORDER BY total DESC) AS rk FROM (SELECT Province, City, SUM(Quantity) AS total FROM %s GROUP BY Province, City) ORDER BY total DESC LIMIT 10",
					 "subjects":[%s],
					 "spec":{"type":"interval","encode":{"x":"City","y":"total","color":"Province"},"axis":{"x":{"labelAutoRotate":true}}}}
				]}`,
				tableName, subjectID, tableName, subjectID, tableName, subjectID))
			return
		}

		switch toolResults {
		case 0:
			writeToolUse(w, "toolu_1", "get_schema", fmt.Sprintf(`{"subject_code":"%s"}`, firstCode(sysText)))
		case 1:
			writeToolUse(w, "toolu_2", "execute_sql", fmt.Sprintf(`{"sql":"SELECT COUNT(*) AS n FROM %s"}`, tableName))
		case 2:
			if subjectCode == "cvms_e2e" {
				writeToolUse(w, "toolu_3", "update_page", fmt.Sprintf(
					`{"charts":[
						{"id":"top-brands","title":"Top10 品牌销量与市占率","width":"full","type":"interval",
						 "sql":"SELECT Brand, qty, ROUND(qty*100.0/SUM(qty) OVER(),2) AS pct FROM (SELECT Brand, SUM(Quantity) AS qty FROM %s GROUP BY Brand) ORDER BY qty DESC LIMIT 10",
						 "subjects":[%s],
						 "spec":{"type":"interval","encode":{"x":"Brand","y":"qty","color":"Brand"},"labels":[{"text":"pct","position":"top"}],"transform":[{"type":"labelTransform.overall"}],"axis":{"x":{"labelAutoRotate":true}}}},
						{"id":"mass-category","title":"吨位段×车辆类别结构","width":"1/2","type":"interval",
						 "sql":"SELECT CASE WHEN GrossMass>=25000 THEN '重型' WHEN GrossMass>=12000 THEN '中型' ELSE '轻/微型' END AS seg, VehicleCategory, SUM(Quantity) AS total FROM %s GROUP BY seg, VehicleCategory ORDER BY total DESC LIMIT 50",
						 "subjects":[%s],
						 "spec":{"type":"interval","encode":{"x":"seg","y":"total","color":"VehicleCategory"},"transform":[{"type":"stackY"}],"legend":{"color":{"position":"top"}}}},
						{"id":"top-cities","title":"Top10 城市销量排名","width":"1/2","type":"interval",
						 "sql":"SELECT Province, City, total, RANK() OVER(ORDER BY total DESC) AS rk FROM (SELECT Province, City, SUM(Quantity) AS total FROM %s GROUP BY Province, City) ORDER BY total DESC LIMIT 10",
						 "subjects":[%s],
						 "spec":{"type":"interval","encode":{"x":"City","y":"total","color":"Province"},"axis":{"x":{"labelAutoRotate":true}}}}
					]}`,
					tableName, subjectID, tableName, subjectID, tableName, subjectID))
			} else {
				writeToolUse(w, "toolu_3", "update_page", fmt.Sprintf(
					`{"charts":[{"id":"c1","title":"区域销售汇总","width":"full","type":"interval","sql":"SELECT region, SUM(amount) AS total FROM %s GROUP BY region","subjects":[%s],"spec":{"type":"interval","encode":{"x":"region","y":"total"}}}]}`,
					tableName, subjectID))
			}
		default:
			if subjectCode == "cvms_e2e" {
				writeText(w, "已为您生成商用车复杂分析图表:Top10品牌市占率、吨位段结构、Top10城市。")
			} else {
				writeText(w, "已为您生成区域销售汇总图表。")
			}
		}
	})
	log.Println("fakellm listening on :9999")
	log.Fatal(http.ListenAndServe(":9999", nil))
}

// extractSystemText 兼容 system 为字符串或 [{type:text,text:...}] 数组。
func extractSystemText(body []byte) string {
	var probe struct {
		System json.RawMessage `json:"system"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || len(probe.System) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(probe.System, &s); err == nil {
		return s
	}
	var arr []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(probe.System, &arr); err == nil {
		parts := make([]string, 0, len(arr))
		for _, a := range arr {
			parts = append(parts, a.Text)
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func firstCode(sysText string) string {
	if m := codeRe.FindStringSubmatch(sysText); len(m) > 1 {
		return m[1]
	}
	return ""
}

// lastUserText 返回最后一条 user 消息的全部文本内容拼接。
func lastUserText(msgs []struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}) string {
	var last string
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		var text string
		for _, c := range m.Content {
			var blk struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(c, &blk) == nil && blk.Type == "text" {
				text += blk.Text
			}
		}
		if text != "" {
			last = text
		}
	}
	return last
}

func writeToolUse(w http.ResponseWriter, id, name, inputJSON string) {
	partial, _ := json.Marshal(inputJSON)
	fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"fake","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
	fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"`+id+`","name":"`+name+`","input":{}}}`)
	fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":`+string(partial)+`}}`)
	fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", `{"type":"content_block_stop","index":0}`)
	fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":1}}`)
	fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", `{"type":"message_stop"}`)
}

func writeText(w http.ResponseWriter, text string) {
	delta, _ := json.Marshal(text)
	fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", `{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"fake","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
	fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":`+string(delta)+`}}`)
	fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", `{"type":"content_block_stop","index":0}`)
	fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`)
	fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", `{"type":"message_stop"}`)
}

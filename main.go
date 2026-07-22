package main

import (
	"cmp"
	"encoding/binary"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"slices"

	"github.com/gorilla/websocket"
)

func main() {
	debug := flag.Bool("debug", false, "sets debug logging")
	flag.Parse()
	lvl := slog.LevelInfo
	if *debug {
		lvl = (slog.LevelDebug)
	}

	logFile, err := os.CreateTemp("", "ffxiv-dps-meter-*.log")
	if err != nil {
		os.Stderr.WriteString("temp log file err: " + err.Error())
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: lvl})))

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	const uri = "ws://localhost:10501/MiniParse"
	slog.Debug("connecting", "uri", uri)

	c, _, err := websocket.DefaultDialer.Dial(uri, nil)
	if err != nil {
		slog.Error("dial", "err", err)
		os.Exit(1)
	}
	defer c.Close()
	slog.Debug("connected", "uri", uri)

	done := make(chan struct{})

	go func() {
		defer close(done)
		// assumption is these can be reused
		am := ACTMsg{}
		cd := ACTMsgCombatData{}
		cmbs := make([]CombatantData, 0, 8)
		msgBuf := &smolBuf{}
		encBuf := &smolBuf{}
		encBuf.WriteString("\033[2J\033[HConnected\nNo Encounter\n")
		for {
			_, rd, err := c.NextReader()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					slog.Error("read", "err", err)
				}
				return
			}
			msgBuf.reset()
			io.Copy(msgBuf, rd)
			err = json.Unmarshal(msgBuf.bs, &am)
			if err != nil {
				slog.Error("json unmarshal", "err", err)
				continue
			}
			switch am.MsgType {
			case "CombatData":
				slog.Debug("combat data", "data", string(msgBuf.bs))
				err = json.Unmarshal(msgBuf.bs, &cd)
				if err != nil {
					slog.Error("json unmarshal", "err", err)
					continue
				}
				enc := cd.Msg.Encounter
				encBuf.reset()
				encBuf.WriteString("\033[2J\033[H\033[32mConnected\033[0m\n")
				enc.fmt(encBuf)
				cmbs = cmbs[:0]
				for _, d := range cd.Msg.Combatant {
					if d.Job == "" {
						// Enemies also on this list, can filter them out by checking if job isn't empty.
						// This probably means pets are removed as well, but I don't play those jobs.
						continue
					} else if d.Job == "Limit Break" {
						d.Job = "LB "
					}
					cmbs = append(cmbs, d)
				}
				slices.SortFunc(cmbs, sortByDamage)
				for _, cmb := range cmbs {
					cmb.fmt(encBuf)
				}
			default:
				slog.Debug("unhandled", "data", string(msgBuf.bs))
			}
			os.Stdout.Write(encBuf.bs)
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			buf := make([]byte, 2)
			binary.BigEndian.PutUint16(buf, uint16(websocket.CloseNormalClosure))
			err := c.WriteMessage(websocket.CloseMessage, buf)
			if err != nil {
				slog.Error("write close", "err", err)
				os.Exit(1)
			}
			return
		}
	}
}

// smolBuf is a simpler bytes.Buffer
type smolBuf struct{ bs []byte }

func (sb *smolBuf) reset() {
	sb.bs = sb.bs[:0]
}
func (sb *smolBuf) Write(b []byte) (int, error) {
	sb.bs = append(sb.bs, b...)
	return len(b), nil
}
func (sb *smolBuf) WriteByte(b byte) error {
	sb.bs = append(sb.bs, b)
	return nil
}
func (sb *smolBuf) WriteString(b string) (int, error) {
	sb.bs = append(sb.bs, b...)
	return len(b), nil
}
func (sb *smolBuf) strsJoin(sep byte, bs ...string) {
	for _, b := range bs {
		sb.bs = append(sb.bs, b...)
		sb.bs = append(sb.bs, sep)
	}
}

type ACTMsg struct {
	MsgType string `json:"msgtype"`
}
type ACTMsgCombatData struct {
	Msg CombatData `json:"msg"`
}
type CombatData struct {
	Encounter Encounter                `json:"Encounter"`
	Combatant map[string]CombatantData `json:"Combatant"`
}
type Encounter struct {
	Title    string `json:"title"`
	Duration string `json:"duration"`
	Damage   string `json:"damage"`
	Dps      string `json:"dps"`
	Deaths   string `json:"deaths"`

	// DurationDupe needed because json unmarshal gets confused for some reason with duration and
	// DURATION both in the JSON
	DurationDupe string `json:"DURATION"`
	// DpsDupe needed because json unmarshal gets confused for some reason with dps and DPS
	// both in the JSON
	DpsDupe string `json:"DPS"`
}

func (e Encounter) fmt(sb *smolBuf) {
	sb.strsJoin(' ', e.Duration, e.Title, "DPS:", padSp(10, e.Dps), "Dmg:", padSp(10, e.Damage), "Deaths:", padSp(2, e.Deaths))
	sb.WriteByte('\n')
}

type CombatantData struct {
	Name             string `json:"name"`
	Damage           string `json:"damage"`
	DamagePct        string `json:"damage%"`
	Dps              string `json:"dps"`
	CritPct          string `json:"crithit%"`
	Deaths           string `json:"deaths"`
	Job              string `json:"Job"`
	DirectHitPct     string `json:"DirectHitPct"`
	CritDirectHitPct string `json:"CritDirectHitPct"`

	// DpsDupe needed because json unmarshal gets confused for some reason with dps and DPS
	// both in the JSON
	DpsDupe string `json:"DPS"`
}

func (c CombatantData) fmt(sb *smolBuf) {
	color := "\033[33m"
	if c.Name != "YOU" {
		switch c.Job {
		case "Whm", "Sch", "Ast", "Sge":
			color = "\033[32m"
		case "Gnb", "War", "Pld", "Drk":
			color = "\033[34m"
		default:
			color = "\033[31m"
		}
	}
	sb.WriteString(color)
	sb.strsJoin(' ', padSp(3, c.DamagePct), c.Job, padSp(20, c.Name))
	sb.WriteByte('\t')
	dps := c.Dps
	if dps == "∞" {
		dps = "Inf"
	}
	sb.strsJoin(' ', "DPS:", padSp(10, dps))
	sb.WriteByte('\t')
	sb.strsJoin(' ', "Dmg:", padSp(10, c.Damage))
	sb.WriteByte('\t')
	sb.strsJoin(' ', "Crt:", padSp(3, c.CritPct))
	sb.WriteByte('\t')
	sb.strsJoin(' ', "DH:", padSp(3, c.DirectHitPct))
	sb.WriteByte('\t')
	sb.strsJoin(' ', "CrtDH:", padSp(3, c.CritDirectHitPct))
	sb.WriteByte('\t')
	sb.strsJoin(' ', "Deaths:", padSp(2, c.Deaths))
	sb.WriteString("\033[0m\n")
}
func sortByDamage(a, b CombatantData) int {
	return cmp.Compare(padZ(10, b.Damage), padZ(10, a.Damage))
}

// Digging into strings.Repeat, for short sequences of spaces, it substrings a constant string of
// spaces. This is to do that directly instead of going through strings.Repeat. For now only need
// at most 20 spaces.
const spaces = "                    "
const zeroes = "00000000000000000000"

func padSp(width int, s string) string {
	if len(s) > width {
		return s
	}
	return spaces[:width-len(s)] + s
}
func padZ(width int, s string) string {
	if len(s) > width {
		return s
	}
	return zeroes[:width-len(s)] + s
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	debug := flag.Bool("debug", false, "sets debug logging")
	flag.Parse()
	if *debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

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
		for {
			_, m, err := c.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
					slog.Error("read", "err", err)
				}
				return
			}
			err = json.Unmarshal(m, &am)
			if err != nil {
				slog.Error("json unmarshal", "err", err)
				continue
			}
			switch am.MsgType {
			case "CombatData":
				err = json.Unmarshal(m, &cd)
				if err != nil {
					slog.Error("json unmarshal", "err", err)
					continue
				}
				fmt.Println("\033[2J\033[H")
				enc := cd.Msg.Encounter
				fmt.Printf("%s %s DPS:%10s Dmg:%10s Kills:%2s Deaths:%2s\n\n", enc.Duration, enc.Title, enc.Dps, enc.Damage, enc.Kills, enc.Deaths)
				cmbs := make([]CombatantData, 0, len(cd.Msg.Combatant))
				for _, d := range cd.Msg.Combatant {
					cmbs = append(cmbs, d)
				}
				slices.SortFunc(cmbs, func(a, b CombatantData) int { return int(b.Damage - a.Damage) })
				for _, cmb := range cmbs {
					fmt.Printf("%3d%% %s:%20s DPS:%10.2f Dmg:%10d Crit:%3d%% DH:%3d%% CritDH:%3d%% Deaths:%2d\n", cmb.DamagePct, cmb.Job, cmb.Name, cmb.Dps, cmb.Damage, cmb.CritPct, cmb.DirectHitPct, cmb.CritDirectHitPct, cmb.Deaths)
				}
			default:
				slog.Debug("unhandled", "data", m)
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				slog.Error("write close", "err", err)
				os.Exit(1)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
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
	Kills    string `json:"kills"`
	Deaths   string `json:"deaths"`

	// DurationDupe needed because json unmarshal gets confused for some reason with duration and DURATION
	// both in the JSON
	DurationDupe string `json:"DURATION"`
	// DpsDupe needed because json unmarshal gets confused for some reason with dps and DPS
	// both in the JSON
	DpsDupe string `json:"DPS"`
}
type CombatantData struct {
	Name             string      `json:"name"`
	Damage           JsonAtoi    `json:"damage"`
	DamagePct        JsonAtoiPct `json:"damage%"`
	Dps              JsonAtod    `json:"dps"`
	CritPct          JsonAtoiPct `json:"crithit%"`
	Deaths           JsonAtoi    `json:"deaths"`
	Job              string      `json:"Job"`
	DirectHitPct     JsonAtoiPct `json:"DirectHitPct"`
	CritDirectHitPct JsonAtoiPct `json:"CritDirectHitPct"`

	// DpsDupe needed because json unmarshal gets confused for some reason with dps and DPS
	// both in the JSON
	DpsDupe string `json:"DPS"`
}

type JsonAtoi int

func (v *JsonAtoi) UnmarshalJSON(data []byte) error {
	var sv string
	if err := json.Unmarshal(data, &sv); err != nil {
		return err
	}

	iv, err := strconv.Atoi(sv)
	if err != nil {
		return err
	}
	*v = JsonAtoi(iv)
	return nil
}

type JsonAtod float64

func (v *JsonAtod) UnmarshalJSON(data []byte) error {
	var sv string
	if err := json.Unmarshal(data, &sv); err != nil {
		return err
	}

	iv, err := strconv.ParseFloat(sv, 64)
	if err != nil {
		return err
	}
	*v = JsonAtod(iv)
	return nil
}

type JsonAtoiPct int

func (v *JsonAtoiPct) UnmarshalJSON(data []byte) error {
	var sv string
	if err := json.Unmarshal(data, &sv); err != nil {
		return err
	}

	iv, err := strconv.Atoi(sv[:len(sv)-1])
	if err != nil {
		return err
	}
	*v = JsonAtoiPct(iv)
	return nil
}

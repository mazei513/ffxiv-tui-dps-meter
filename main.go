package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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
				fmt.Printf("%s Dur:%s DPS:%s Dmg:%s Kills:%s Deaths:%s\n\n", enc.Title, enc.Duration, enc.Dps, enc.Damage, enc.Kills, enc.Deaths)
				for name, d := range cd.Msg.Combatant {
					fmt.Printf("%s:%s %%:%s DPS:%s Dmg:%s Crit:%s DH:%s CritDH:%s Deaths:%s\n", d.Job, name, d.DamagePct, d.Dps, d.Damage, d.CritPct, d.DirectHitPct, d.CritDirectHitPct, d.Deaths)
				}
			default:
				slog.Debug("unhandled", "data", m)
			}
		}
	}()

	for {
		select {
		case <-done:
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
	Title           string `json:"title"`
	Duration        string `json:"duration"`
	Damage          string `json:"damage"`
	Dps             string `json:"dps"`
	Kills           string `json:"kills"`
	Deaths          string `json:"deaths"`
	CurrentZoneName string `json:"CurrentZoneName"`

	// DurationDupe needed because json unmarshal gets confused for some reason with duration and DURATION
	// both in the JSON
	DurationDupe string `json:"DURATION"`
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
}

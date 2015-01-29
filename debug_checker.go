package main

import "net/http"
import "strconv"
import "fmt"

type DebugChecker struct {
	status PowerStatus
}

func (c *DebugChecker) Init(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		q := req.URL.Query()
		errors := map[string]error{}
		if val, ok := q["plug"]; ok {
			if val[0] != "0" {
				c.status.Charging = true
			} else {
				c.status.Charging = false
			}
		}

		if val, ok := q["level"]; ok {
			level, err := strconv.ParseInt(val[0], 10, 32)
			if err == nil {
				c.status.ChargeLevel = float32(level) / 1000
			} else {
				errors["level"] = err
			}
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)

		if len(errors) == 0 {
			fmt.Fprintln(w, "ok")
		} else {
			for name, err := range errors {
				fmt.Fprintf(w, "%s: %s\n", name, err)
			}
		}
	})

	go http.ListenAndServe(addr, mux)
	return nil
}

func (c *DebugChecker) Check() (*PowerStatus, error) {
	st := c.status
	return &st, nil
}

func (c *DebugChecker) Stop() {}

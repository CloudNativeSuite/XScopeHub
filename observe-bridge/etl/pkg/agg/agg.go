package agg

import (
        "bytes"
        "sync"

        "github.com/xscopehub/xscopehub/etl/pkg/window"
)

// Record represents a processed record destined for a tenant/window bucket.
type Record struct {
        Tenant string
        Window window.Window
        Data   []byte
}

var (
        mu   sync.Mutex
        // buckets holds pending aggregates organised by tenant and window.
        // Data for each window is stored as a list of byte slices which will be
        // joined on drain. This allows delaying writes without losing inputs.
        buckets = make(map[string]map[window.Window][][]byte)
)

// Feed ingests a record into the aggregator. Each record is stored once in a
// tenant/window bucket awaiting flush.
func Feed(rec Record) error {
        mu.Lock()
        defer mu.Unlock()
        tw, ok := buckets[rec.Tenant]
        if !ok {
                tw = make(map[window.Window][][]byte)
                buckets[rec.Tenant] = tw
        }
        tw[rec.Window] = append(tw[rec.Window], rec.Data)
        return nil
}

// Drain returns aggregated results grouped by tenant and window. After drain
// the internal buffers are cleared so each record is written exactly once.
func Drain() map[string]map[window.Window][]byte {
        mu.Lock()
        defer mu.Unlock()

        out := make(map[string]map[window.Window][]byte, len(buckets))
        for tenant, wmap := range buckets {
                out[tenant] = make(map[window.Window][]byte, len(wmap))
                for w, slices := range wmap {
                        out[tenant][w] = bytes.Join(slices, []byte("\n"))
                }
        }
        buckets = make(map[string]map[window.Window][][]byte)
        return out
}


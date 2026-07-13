// This gets compiled to WebAssembly and runs in the browser. It takes a
// batch of bids plus the user's geo and picks a winner using a standard
// second-price auction, the same mechanism most RTB exchanges use.
//
// The bigger idea behind this whole demo: instead of shipping bid data out
// to some ad server and waiting on a response, the auction runs locally,
// on the client, with nothing but the signals it already has. That's the
// same core idea Chrome's Protected Audience API is built on, minus the
// actual sandboxing (see README for why that part can't be faked).
//
// Went with Go here instead of Rust or AssemblyScript mostly because the
// JSON handling and the syscall/js bridge are both in the standard
// library, so there's no extra tooling to fight with for a prototype.
package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"
)

// Matches the JSON coming back from the FastAPI endpoint. Using explicit
// json tags here instead of relying on Go's case-insensitive field
// matching, since we don't want a silent break if someone renames a field on the
// backend later.
type Bid struct {
	BidID       string  `json:"bid_id"`
	BuyerName   string  `json:"buyer_name"`
	BidCPM      float64 `json:"bid_cpm"`
	CreativeURL string  `json:"creative_url"`
	TargetGeo   string  `json:"target_geo"`
}

// Second-price auction, geo-filtered first.
//
// Winner is whoever bid highest among eligible bids. They pay one cent
// above the second-highest eligible bid, capped at their own bid (you
// should never charge someone more than they actually offered). If
// there's only one eligible bidder, there's nothing to clear against, so
// they just pay what they bid. Treating that case as "$0.01" would be a
// bug, not an edge case worth shrugging off.
func runSecondPriceAuction(bids []Bid, userGeo string) (winner Bid, clearingPrice float64, err error) {
	eligible := make([]Bid, 0, len(bids))
	for _, b := range bids {
		if b.TargetGeo == "ALL" || b.TargetGeo == userGeo {
			eligible = append(eligible, b)
		}
	}

	if len(eligible) == 0 {
		return Bid{}, 0, fmt.Errorf("no eligible bids for geo %q", userGeo)
	}

	// Single pass to find the top two by CPM instead of sorting the whole
	// slice. Doesn't matter much at this scale but it's the right habit
	// for something that's meant to run per-impression.
	var highest, secondHighest Bid
	highestSet, secondSet := false, false

	for _, b := range eligible {
		switch {
		case !highestSet || b.BidCPM > highest.BidCPM:
			secondHighest, secondSet = highest, highestSet
			highest, highestSet = b, true
		case !secondSet || b.BidCPM > secondHighest.BidCPM:
			secondHighest, secondSet = b, true
		}
	}

	winner = highest
	if secondSet {
		clearingPrice = secondHighest.BidCPM + 0.01
		if clearingPrice > winner.BidCPM {
			clearingPrice = winner.BidCPM // never charge more than they bid
		}
	} else {
		clearingPrice = winner.BidCPM // lone bidder, nothing to clear against
	}

	return winner, clearingPrice, nil
}

func buildAuctionResult(winner Bid, clearingPrice float64) js.Value {
	return js.ValueOf(map[string]interface{}{
		"success":       true,
		"winningBuyer":  winner.BuyerName,
		"creativeUrl":   winner.CreativeURL,
		"clearingPrice": clearingPrice,
		"winningBidId":  winner.BidID,
	})
}

func buildAuctionError(message string) js.Value {
	return js.ValueOf(map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

// Exported to JS as runAuctionEngine(bidsJson, userGeo). Bids come in as
// a raw JSON string rather than a JS array. Walking a JS object
// field-by-field through syscall/js is noticeably slower than just handing
// the string to encoding/json and letting Go parse it in one shot. The
// frontend already has the bids as JSON text straight from the fetch
// response anyway, so nothing extra needs to happen on that side either.
//
// Kept this synchronous on purpose. No Promise, no callback, the auction
// math is cheap enough that blocking briefly is fine, and it keeps the
// benchmark timing on the JS side honest (start the clock, call the
// function, stop the clock, done).
func runAuctionEngine(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return buildAuctionError("runAuctionEngine requires (bidsJson, userGeo) arguments")
	}

	bidsJSON := args[0].String()
	userGeo := args[1].String()

	var bids []Bid
	if err := json.Unmarshal([]byte(bidsJSON), &bids); err != nil {
		// Bad payload from the backend shouldn't take the whole page down,
		// return it as a normal failure the UI already knows how to render.
		return buildAuctionError(fmt.Sprintf("failed to parse bids payload: %v", err))
	}

	if len(bids) == 0 {
		return buildAuctionError("no bids supplied to auction engine")
	}

	winner, clearingPrice, err := runSecondPriceAuction(bids, userGeo)
	if err != nil {
		return buildAuctionError(err.Error())
	}

	return buildAuctionResult(winner, clearingPrice)
}

func main() {
	js.Global().Set("runAuctionEngine", js.FuncOf(runAuctionEngine))

	// A Go Wasm module exits the moment main() returns, but our exported
	// function only gets called later, from JS. select{} just blocks
	// forever so the runtime stays alive for the life of the page. Setting
	// this flag right before it lets the frontend know the function is
	// actually registered and ready to call. Instantiation finishing and
	// main() actually running aren't quite the same moment.
	js.Global().Set("__auctionEngineReady", true)

	select {}
}

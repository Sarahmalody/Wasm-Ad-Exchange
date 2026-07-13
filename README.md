# Edge Auctioning & Real-Time Bidding Signals Engine

A local simulation of on-device programmatic auctions, the model behind
Chrome's Protected Audience API. Bidding signals are fetched once from a
lightweight backend; the actual auction (a second-price auction with geo
eligibility filtering) runs entirely client-side in WebAssembly, so no
auction outcome or bid data ever leaves the browser.

## Architecture

```
backend/    FastAPI service returning mock DSP bidding signals
engine/     Go source compiled to WebAssembly (the actual auction logic)
frontend/   Static page that wires the two together and benchmarks the auction
```

## Running it

**1. Start the backend**

```bash
cd backend
python3 -m venv venv && source venv/bin/activate
pip install -r requirements.txt
python main.py
# Serving on http://localhost:8000
```

**2. Compile the Wasm engine**

```bash
cd engine
./build.sh
cp auction.wasm wasm_exec.js ../frontend/
```

(Requires Go 1.21+. See `engine/build.sh` for the manual commands if you'd
rather run them individually.)

**3. Serve the frontend**

Wasm's streaming instantiation requires a real HTTP origin (not `file://`),
so serve it with any static server:

```bash
cd frontend
python3 -m http.server 5500
# Open http://localhost:5500
```

Change the geo dropdown to see the auction re-run against the cached bid
set, or hit "Run Auction" to simulate a fresh pageview and refetch signals.

## Why this design

- **Backend does I/O, Wasm does math.** The network round-trip (fetching
  signals) happens once per pageview; the auction itself is a pure,
  synchronous computation with zero network dependency, which is the
  entire point of moving auctions on-device.
- **Second-price with a self-bid ceiling.** The winner pays one cent above
  the runner-up, capped at their own bid, and a sole eligible bidder pays
  their own price rather than an arbitrary $0.01. Both are easy edge
  cases to get subtly wrong.
- **JSON string in, not JS object walking.** The Wasm boundary is the
  slowest part of this stack; passing bids as a JSON string and letting
  Go's `encoding/json` parse them in one shot avoids a field-by-field
  `syscall/js` walk.

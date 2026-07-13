"""
Bidding signals API for the edge auction demo.

This plays the role of the "seller-side" endpoint a publisher's page would
hit to grab the latest bidding signals before running an auction in the
browser. Kept it deliberately dumb and fast. In a real setup this would
be backed by a bid cache that DSPs push into asynchronously, since the
browser shouldn't ever be blocked on a live RTB call. The whole point of
on-device auctions is that the slow part (figuring out roughly who might
want this impression and for how much) happens ahead of time, and the
actual auction math runs locally with no round trip.
"""

import random
import uuid
from datetime import datetime, timezone

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field

app = FastAPI(
    title="Edge Auction Signals API",
    description="Serves mock DSP bidding signals for on-device auction simulation.",
    version="1.0.0",
)

# Frontend runs on a different origin during local dev, so CORS needs to
# be wide open here. Would obviously lock this down to the publisher's
# real domain before this ever saw production traffic.
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["GET"],
    allow_headers=["*"],
)


class BidSignal(BaseModel):
    """One DSP's bid for this impression: buyer, price, creative, geo.
    bid_id exists so the frontend can track which bid won without relying
    on array position."""

    bid_id: str
    buyer_name: str
    bid_cpm: float = Field(..., description="Bid price in USD, cost per mille")
    creative_url: str
    target_geo: str = Field(..., description="ISO country code this bid is valid for, or 'ALL'")


# Small roster of fake DSPs. CPMs are biased per-partner instead of pulled
# from one flat random range across the board, since a uniform distribution is
# the fastest way to make mock data look obviously fake.
_DEMAND_PARTNERS = [
    {"buyer_name": "Northwind Retail Co-op", "cpm_range": (2.10, 6.75), "geos": ["US", "CA"]},
    {"buyer_name": "Solace Travel Group", "cpm_range": (4.50, 9.25), "geos": ["US", "GB", "AU"]},
    {"buyer_name": "Fernwood Financial", "cpm_range": (8.00, 14.50), "geos": ["US"]},
    {"buyer_name": "Kite & Compass Apparel", "cpm_range": (1.75, 4.90), "geos": ["ALL"]},
    {"buyer_name": "Basalt Auto Group", "cpm_range": (6.20, 11.80), "geos": ["US", "DE"]},
    {"buyer_name": "Perigon Streaming", "cpm_range": (3.30, 7.60), "geos": ["ALL"]},
    {"buyer_name": "Hollow Creek Outfitters", "cpm_range": (0.90, 3.25), "geos": ["CA", "GB"]},
]

_PLACEHOLDER_CREATIVE_BASE = "https://picsum.photos/seed"


def _generate_bid(partner: dict) -> BidSignal:
    low, high = partner["cpm_range"]
    cpm = round(random.uniform(low, high), 2)  # CPMs price to the cent, not to arbitrary decimals
    geo = random.choice(partner["geos"])

    # Same creative per buyer across refreshes (seeded on their name) so
    # the page doesn't feel completely random every reload, even though
    # the bid price itself changes each time.
    seed = partner["buyer_name"].replace(" ", "-").lower()
    creative_url = f"{_PLACEHOLDER_CREATIVE_BASE}/{seed}/480/300"

    return BidSignal(
        bid_id=str(uuid.uuid4()),
        buyer_name=partner["buyer_name"],
        bid_cpm=cpm,
        creative_url=creative_url,
        target_geo=geo,
    )


@app.get("/api/bids", response_model=list[BidSignal])
def get_bids(count: int = 6):
    """Returns a fresh batch of mock bids. Defaults to 6 rather than every
    partner every time, since not every DSP has demand for every impression in
    real life, so a partial, slightly randomized set is more honest than
    a fixed full roster on every request."""

    # Clamp instead of erroring if someone asks for more than we have
    # partners for, since this is a non-critical mock endpoint and a 500 here
    # would be overkill.
    sample_size = max(1, min(count, len(_DEMAND_PARTNERS)))
    chosen_partners = random.sample(_DEMAND_PARTNERS, k=sample_size)

    return [_generate_bid(partner) for partner in chosen_partners]


@app.get("/api/health")
def health_check():
    """Liveness probe so the frontend can fail fast with a clear message
    instead of hanging on a dead backend."""
    return {"status": "ok", "timestamp": datetime.now(timezone.utc).isoformat()}


if __name__ == "__main__":
    # Local dev entrypoint. In production this would run under uvicorn
    # workers behind a real ASGI setup, not the dev reloader.
    import uvicorn

    uvicorn.run("main:app", host="0.0.0.0", port=8000, reload=True)

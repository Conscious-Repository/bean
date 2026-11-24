"""Check a synthetic reader replay from stdin, including the evidence for each decision."""
import json
import sys


def check(report):
    assert report["format"] == "bean-coverage/1" and report["cohort"] == "male-20-29"
    assert report["total"] == len(report["rows"]) == len({r["id"] for r in report["rows"]}) == 5
    assert report["source_covered"] == 4
    assert report["counts"] == dict(placed=1, bounded=1, unreported=1, unmatched_numeric=2, text=0, other=0)
    pack, = report["selection"]["packs"]
    guide, = report["selection"]["guides"]
    for r in report["rows"]:
        refs = r.get("assay_sources", [])
        if r["observed_fields"]["context.panel"] == "Other panel":
            assert r["value"] == {"n": 1.6} and r["status"] == "unmatched_numeric" and not refs
            decline, = r["declines"]
            rule, = decline["alternatives"]
            assert decline["sha256"] == pack["sha256"] and rule["different"] == ["context.panel"]
            assert rule["required"]["context.panel"] == "COMPREHENSIVE METABOLIC PANEL"
        else:
            ref, = refs
            assert ref["sha256"] == guide["sha256"] and ref["url"].startswith("https://")
        if r["status"] == "placed":
            m, = r["matches"]
            assert r["value"] == {"n": 1.6} and m["evidence"]["required"]["value.unit"] is None
            assert (m["measure"], m["n"], m["percentile"], m["sha256"]) == ("AG", 346, 64.4, pack["sha256"])
        else:
            assert r["matches"] == [] and r["reason"]
        if r["status"] == "bounded":
            assert r["value"] == {"s": "<1.6"}
        if r["status"] == "unreported":
            assert r["value"] == {"n": None}
        if r["type"] == "lab.cortisol-total":
            assert r["status"] == "unmatched_numeric" and refs and not r.get("declines")


if __name__ == "__main__":
    check(json.load(sys.stdin))
    print("Verified all five synthetic readings.")

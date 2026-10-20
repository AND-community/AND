import { test } from "node:test";
import assert from "node:assert/strict";
import { Grup } from "../../types";
import { inviteCandidates } from "./groupOps";

const grup: Grup = {
  id: "g1",
  ad: "ekip",
  sir: "",
  kurucu_ad: "ben",
  kurucu_peer: "12D3KooWBen",
  uyeler: [{ ad: "ben", peer_id: "12D3KooWBen" }, { ad: "aziz", peer_id: "12D3KooWAziz" }],
  olusturma_ts: "",
};

test("inviteCandidates AND düğümü işaretliyse yalnızca üye olmayan AND düğümlerini önerir", () => {
  const peers = [
    { peer_id: "12D3KooWDht1", addrs: [] },
    { peer_id: "12D3KooWAziz", addrs: [], and: true },
    { peer_id: "12D3KooWBob", addrs: [], and: true },
    { peer_id: "12D3KooWDht2", addrs: [] },
  ];
  assert.deepEqual(
    inviteCandidates(peers, grup).map((p) => p.peer_id),
    ["12D3KooWBob"]
  );
});

test("inviteCandidates and alanı olmayan eski AND'da üye olmayan tüm eşleri bırakır", () => {
  const peers = [
    { peer_id: "12D3KooWDht1", addrs: [] },
    { peer_id: "12D3KooWBen", addrs: [] },
    { peer_id: "12D3KooWBob", addrs: [] },
  ];
  assert.deepEqual(
    inviteCandidates(peers, grup).map((p) => p.peer_id),
    ["12D3KooWDht1", "12D3KooWBob"]
  );
});

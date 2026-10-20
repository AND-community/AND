import { Uye } from "../../types";
import { LiveClient } from "../liveClient";

/** Bağlı canlı istemcinin kimliğini store.go'daki Uye biçimine çevirir. */
export async function fetchSelfUye(client: LiveClient): Promise<Uye | undefined> {
  try {
    const id = await client.getIdentity();
    return { ad: id.name, peer_id: id.peer_id };
  } catch {
    return undefined;
  }
}

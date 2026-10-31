# Change notes folded into SPEC.md

SPEC.md v0.5 includes every change note below. This file keeps the list for readers of older drafts. The reasons for each note are in docs/DECISIONS.md.

v0.4 applied change notes C1 to C20: seed-only rotation, a fixed envelope, verify before copy in sync, and no erasure on a timer. v0.5 adds four change notes:

- C21: secp256k1 (Nostr) signing keys (4.4, 4.5).
- C22: restore on a new host (5.10).
- C23: superseded records are not in duplicate groups (6.3 step 12).
- C24: the protocol name is bean. The HKDF labels (4.7), the dedupe ref label (4.8), the blob label (4.9), the reserved types (3.3) and the wrapper `alt` tag (4.5) say `bean`. Keys, ids and vectors from builds before C24 do not match.

On 2026-10-30 the audit of `b7a1626` added three change notes. They change which records are valid or erased. Records, ids, signatures, keys and the wire labels do not change.

- C25: erasure follows record files, not validity. `purge` and `gc` erase only the blobs of the records that they erase, and only when no remaining record file names the blob. `gc` refuses on a too-new or frozen identity (7.4 step 4, 7.6 step 5, 7.7 step 5).
- C26: a retired key writes nothing valid. A succession seals every valid record of the outgoing key, not only the destructive ones. A record of an older chain key that the next succession does not seal is invalid (5.4, 5.5 step 6, 6.3 step 7, 6.5). Benjamin decision.
- C27: duplicate groups include the author (6.3 step 12). A record of one key is never a duplicate of a record of another key.
- C28: seal continuations. A rotation that must seal more ids than one succession holds writes the rest in `bean.seal` records, signed by the same recovery key (3.3, 5.4, 6.3 steps 5b and 7). The succession says how many there are in `value.parts`. Only a complete set seals anything. A succession without `value.parts` is unchanged, so logs that never needed a continuation do not change. Benjamin decision, 2026-10-30.
- K1: the too-new gate key set (8.2). Benjamin decision, 2026-10-30. Only the head key or a recovery key that a chain node commits to can make an identity too new. Other well-signed records give `ungated: <id>`. This is the K1 decision of draft v0.7, which the repository had not taken over. It changes results only for logs with a `bean.*` record of an unknown `v`. Vector V22.
- C29: the 24 words go past a halt by the head key (5.5 step 4a, 8.2). A thief with the current signing key could halt the identity for good, because `rotate` refused a too-new identity. Now `rotate` and `restore` go past a halt when every record that halts the identity is signed by its head key. The succession retires that key, so on every verifier the records become `ungated` and the halt ends. A halt in which a recovery key took part still needs an update. The verifier rules do not change. Benjamin decision, 2026-10-30: fix now.

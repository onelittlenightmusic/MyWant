/**
 * wantHashCache — module-level ETag cache for smart polling.
 *
 * Maintains:
 *   collectionETag  — the collection-level hash from GET /api/v1/wants/hashes
 *   wantETags       — per-want hash, used as If-None-Match on GET /api/v1/wants/{id}
 *
 * smartPollWants() executes the 3-phase polling strategy:
 *   1. GET /api/v1/wants/hashes  (If-None-Match: collectionETag)
 *      → 304  → nothing changed, return early
 *      → 200  → diff against cached hashes
 *   2. Identify changed IDs and removed IDs
 *   3. GET /api/v1/wants/{id} in parallel for changed IDs (If-None-Match per want)
 *      → 304  → skip
 *      → 200  → call patchWant()
 *      removed IDs → call removeWantById()
 */

import { apiClient } from '@/api/client';
import type { Want } from '@/types/want';

// Actions are registered by wantStore after creation to avoid circular imports
type PatchWantFn = (updated: Want) => void;
type RemoveWantByIdFn = (id: string) => void;

let _patchWant: PatchWantFn | null = null;
let _removeWantById: RemoveWantByIdFn | null = null;

/** Called from wantStore once the store is created. */
export function registerWantCacheActions(
  patchWant: PatchWantFn,
  removeWantById: RemoveWantByIdFn
) {
  _patchWant = patchWant;
  _removeWantById = removeWantById;
}

// Module-level ETag state
let collectionETag: string | undefined = undefined;
const wantETags = new Map<string, string>(); // id → hash

/**
 * Seed the ETag cache from the initial full fetchWants() response.
 * Call this right after the first successful want list load.
 */
export function seedWantETags(wants: Array<{ metadata?: { id?: string }; hash?: string }>) {
  wants.forEach(w => {
    const id = w.metadata?.id;
    if (id && w.hash) wantETags.set(id, w.hash);
  });
}

/**
 * Invalidate the collection ETag (e.g. after create/delete/update mutation)
 * so the next smartPollWants() forces a full re-check.
 */
export function invalidateCollectionETag() {
  collectionETag = undefined;
}

/**
 * 3-phase smart polling for wants.
 * An in-flight guard prevents concurrent executions.
 */
let polling = false;

export async function smartPollWants(): Promise<void> {
  if (polling) return;
  if (!_patchWant || !_removeWantById) return; // not yet registered

  polling = true;
  try {
    // Phase 1: lightweight hash check
    const hashesResponse = await apiClient.listWantHashes(collectionETag);

    // 304 → collection unchanged, nothing to do
    if (hashesResponse === null) return;

    collectionETag = hashesResponse.collection_hash;

    // Phase 2: diff
    const incomingIds = new Set(hashesResponse.wants.map(e => e.id));
    const removedIds = [...wantETags.keys()].filter(id => !incomingIds.has(id));
    const changedEntries = hashesResponse.wants.filter(
      e => wantETags.get(e.id) !== e.hash
    );

    // Phase 3: parallel fetch of only the changed wants
    await Promise.all(
      changedEntries.map(async ({ id, hash }) => {
        const result = await apiClient.getWantConditional(id, wantETags.get(id));
        if (result.data !== null) {
          _patchWant!(result.data);
        }
        wantETags.set(id, hash);
      })
    );

    removedIds.forEach(id => {
      _removeWantById!(id);
      wantETags.delete(id);
    });
  } catch (err) {
    console.error('[smartPollWants] error:', err);
  } finally {
    polling = false;
  }
}

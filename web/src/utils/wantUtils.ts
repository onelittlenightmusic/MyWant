import { Want, UpdateWantRequest } from '@/types/want';

/**
 * Update want parameters
 */
export async function updateWantParameters(
  wantId: string,
  want: Want,
  newParams: Record<string, any>,
  updateWantFn: (id: string, request: UpdateWantRequest) => Promise<void>
): Promise<void> {
  await updateWantFn(wantId, {
    metadata: {
      name: want.metadata?.name,
      type: want.metadata?.type,
      labels: want.metadata?.labels
    },
    spec: {
      ...want.spec,
      params: newParams
    }
  });
}

/**
 * Update want scheduling
 */
export async function updateWantScheduling(
  wantId: string,
  want: Want,
  newWhen: Array<{ at?: string; every: string }>,
  updateWantFn: (id: string, request: UpdateWantRequest) => Promise<void>
): Promise<void> {
  await updateWantFn(wantId, {
    metadata: {
      name: want.metadata?.name,
      type: want.metadata?.type,
      labels: want.metadata?.labels
    },
    spec: {
      ...want.spec,
      when: newWhen
    }
  });
}

/**
 * Update want labels via API endpoints
 */
export async function updateWantLabels(
  wantId: string,
  oldLabels: Record<string, string>,
  newLabels: Record<string, string>
): Promise<void> {
  // Determine what changed
  const added = Object.keys(newLabels).filter(key => !oldLabels[key]);
  const removed = Object.keys(oldLabels).filter(key => !newLabels[key]);
  const updated = Object.keys(newLabels).filter(key =>
    oldLabels[key] && oldLabels[key] !== newLabels[key]
  );

  // Remove deleted labels
  for (const key of removed) {
    await fetch(`/api/v1/wants/${wantId}/labels/${key}`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' }
    });
  }

  // Add new labels
  for (const key of added) {
    await fetch(`/api/v1/wants/${wantId}/labels`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key, value: newLabels[key] })
    });
  }

  // Update changed labels (delete old + add new)
  for (const key of updated) {
    await fetch(`/api/v1/wants/${wantId}/labels/${key}`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' }
    });
    await fetch(`/api/v1/wants/${wantId}/labels`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key, value: newLabels[key] })
    });
  }
}

/**
 * Update want dependencies via API endpoints
 */
export async function updateWantDependencies(
  wantId: string,
  oldUsing: Array<Record<string, string>>,
  newUsing: Array<Record<string, string>>
): Promise<void> {
  // Simple approach: remove all old, add all new
  // Remove all old dependencies
  for (const dep of oldUsing) {
    const key = Object.keys(dep)[0];
    if (key) {
      await fetch(`/api/v1/wants/${wantId}/using/${key}`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' }
      });
    }
  }

  // Add all new dependencies
  for (const dep of newUsing) {
    const [key, value] = Object.entries(dep)[0];
    if (key) {
      await fetch(`/api/v1/wants/${wantId}/using`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key, value })
      });
    }
  }
}

export function dropLabelOnWant(
  wantId: string,
  e: React.DragEvent,
  onSuccess?: (wantId: string) => void
) {
  try {
    const labelData = e.dataTransfer.getData('application/json');
    if (!labelData) return;
    const { key, value } = JSON.parse(labelData);
    fetch(`/api/v1/wants/${wantId}/labels`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key, value }),
    })
      .then(response => { if (response.ok) onSuccess?.(wantId); })
      .catch(error => console.error('Error dropping label:', error));
  } catch (error) {
    console.error('Error dropping label:', error);
  }
}

import { useState } from 'react';
import { useWantStore } from '@/stores/wantStore';

export function useCardOverlay(id: string | null) {
  const {
    quickActionsWantId,
    setQuickActionsWantId,
    deleteConfirmWantId,
    setDeleteConfirmWantId,
  } = useWantStore();

  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  const showQuickActions = quickActionsWantId === id;
  const showDeleteOverlay = showDeleteConfirm || deleteConfirmWantId === id;

  const closeQuickActions = () => setQuickActionsWantId(null);

  const openDeleteConfirm = () => setShowDeleteConfirm(true);

  const closeDeleteConfirm = () => {
    setShowDeleteConfirm(false);
    setDeleteConfirmWantId(null);
  };

  const confirmDelete = () => {
    setShowDeleteConfirm(false);
    setDeleteConfirmWantId(null);
    setQuickActionsWantId(null);
  };

  return {
    showQuickActions,
    showDeleteOverlay,
    closeQuickActions,
    openDeleteConfirm,
    closeDeleteConfirm,
    confirmDelete,
    setQuickActionsWantId,
  };
}

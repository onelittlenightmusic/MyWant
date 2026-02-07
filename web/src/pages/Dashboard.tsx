import React, { useState, useEffect, useRef, useMemo } from 'react';
import { RefreshCw, ChevronDown } from 'lucide-react';
import { WantExecutionStatus, Want } from '@/types/want';
import { useWantStore } from '@/stores/wantStore';
import { useWantTypeStore } from '@/stores/wantTypeStore';
import { useRecipeStore } from '@/stores/recipeStore';
import { usePolling } from '@/hooks/usePolling';
import { useHierarchicalKeyboardNavigation } from '@/hooks/useHierarchicalKeyboardNavigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { useRightSidebarExclusivity } from '@/hooks/useRightSidebarExclusivity';
import { StatusBadge } from '@/components/common/StatusBadge';
import { classNames, truncateText } from '@/utils/helpers';
import { addLabelToRegistry } from '@/utils/labelUtils';
import { generateUniqueWantName } from '@/utils/nameGenerator';
import { apiClient } from '@/api/client';
import { Recommendation, ConfigModifications } from '@/types/interact';
import { DraftWant, wantToDraft, isDraftWant } from '@/types/draft';

// Components
import { Layout } from '@/components/layout/Layout';
import { Header } from '@/components/layout/Header';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { WantGrid } from '@/components/dashboard/WantGrid';
import { WantMinimap } from '@/components/dashboard/WantMinimap';
import { WantForm } from '@/components/forms/WantForm';
import { WantDetailsSidebar } from '@/components/sidebar/WantDetailsSidebar';
import { SummarySidebarContent } from '@/components/sidebar/SummarySidebarContent';
import { WantBatchControlPanel } from '@/components/dashboard/WantBatchControlPanel';
import { Toast } from '@/components/notifications';
import { ConfirmationBubble } from '@/components/notifications/ConfirmationBubble';
import { DragOverlay } from '@/components/dashboard/DragOverlay';


export const Dashboard: React.FC = () => {
  const { 
    wants, loading, error, fetchWants, deleteWant, deleteWants, 
    suspendWant, resumeWant, stopWant, startWant, clearError, 
    draggingTemplate, setDraggingTemplate 
  } = useWantStore();

  const sidebar = useRightSidebarExclusivity<Want>();
  const [editingWant, setEditingWant] = useState<Want | null>(null);
  const [lastSelectedWantId, setLastSelectedWantId] = useState<string | null>(null);
  const [deleteWantState, setDeleteWantState] = useState<Want | null>(null);
  const [showDeleteConfirmation, setShowDeleteConfirmation] = useState(false);
  const [isDeletingWant, setIsDeletingWant] = useState(false);
  const [showBatchConfirmation, setShowBatchConfirmation] = useState(false);
  const [batchAction, setBatchAction] = useState<'start' | 'stop' | 'delete' | null>(null);
  const [isBatchProcessing, setIsBatchProcessing] = useState(false);
  const [reactionWantState, setReactionWantState] = useState<Want | null>(null);
  const [showReactionConfirmation, setShowReactionConfirmation] = useState(false);
  const [reactionAction, setReactionAction] = useState<'approve' | 'deny' | null>(null);
  const [isSubmittingReaction, setIsSubmittingReaction] = useState(false);
  const [sidebarMinimized, setSidebarMinimized] = useState(true);
  const [sidebarInitialTab, setSidebarInitialTab] = useState<'settings' | 'results' | 'logs' | 'agents'>('settings');
  const [expandedParents, setExpandedParents] = useState<Set<string>>(new Set());
  const [selectedLabel, setSelectedLabel] = useState<{ key: string; value: string } | null>(null);
  const [labelOwners, setLabelOwners] = useState<Want[]>([]);
  const [labelUsers, setLabelUsers] = useState<Want[]>([]);
  const [allLabels, setAllLabels] = useState<Map<string, Set<string>>>(new Map());
  const [isSelectMode, setIsSelectMode] = useState(false);
  const [selectedWantIds, setSelectedWantIds] = useState<Set<string>>(new Set());
  const [isExporting, setIsExporting] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [notificationMessage, setNotificationMessage] = useState<string | null>(null);
  const [isNotificationVisible, setIsNotificationVisible] = useState(false);
  const [activeDraftId, setActiveDraftId] = useState<string | null>(null);
  const [deleteDraftState, setDeleteDraftState] = useState<DraftWant | null>(null);
  const [showDeleteDraftConfirmation, setShowDeleteDraftConfirmation] = useState(false);

  // Global Drag and Drop state
  const [isGlobalDragOver, setIsGlobalDragOver] = useState(false);
  const [dragCounter, setDragCounter] = useState(0);

  // Minimap state
  const [minimapOpen, setMinimapOpen] = useState(true); // Desktop default: true

  const drafts = useMemo(() => wants.filter(isDraftWant).map(wantToDraft).filter((d): d is DraftWant => d !== null), [wants]);
  const regularWants = useMemo(() => wants.filter(w => !isDraftWant(w)), [wants]);
  const [recommendations, setRecommendations] = useState<Recommendation[]>([]);
  const [selectedRecommendation, setSelectedRecommendation] = useState<Recommendation | null>(null);
  const [showRecommendationForm, setShowRecommendationForm] = useState(false);
  const [gooseProvider, setGooseProvider] = useState<string>('claude-code');
  const hasThinkingDraft = drafts.some(d => d.isThinking);

  const showNotification = (message: string) => { setNotificationMessage(message); setIsNotificationVisible(true); };
  const dismissNotification = () => { setIsNotificationVisible(false); setNotificationMessage(null); };

  const selectedWant = useMemo(() => {
    if (!sidebar.selectedItem) return null;
    const wantId = sidebar.selectedItem.metadata?.id || sidebar.selectedItem.id;
    return wants.find(w => (w.metadata?.id === wantId) || (w.id === wantId)) || sidebar.selectedItem;
  }, [sidebar.selectedItem, wants]);

  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilters, setStatusFilters] = useState<WantExecutionStatus[]>([]);
  const [filteredWants, setFilteredWants] = useState<Want[]>([]);
  const flattenedWants = filteredWants.flatMap((pw: any) => [pw, ...(pw.children || [])]);
  const hierarchicalWants = flattenedWants.map(w => ({ id: w.metadata?.id || w.id || '', parentId: w.metadata?.ownerReferences?.[0]?.id }));
  const currentHierarchicalWant = selectedWant ? { id: selectedWant.metadata?.id || selectedWant.id || '', parentId: selectedWant.metadata?.ownerReferences?.[0]?.id } : null;
  const [headerState, setHeaderState] = useState<{ autoRefresh: boolean; loading: boolean; status: WantExecutionStatus } | null>(null);

  const fetchLabels = async () => {
    try {
      const response = await fetch('http://localhost:8080/api/v1/labels');
      if (response.ok) {
        const data = await response.json();
        const labelsMap = new Map<string, Set<string>>();
        if (data.labelValues) {
          for (const [key, valuesArray] of Object.entries(data.labelValues)) {
            if (!labelsMap.has(key)) labelsMap.set(key, new Set());
            if (Array.isArray(valuesArray)) {
              (valuesArray as any[]).forEach(item => {
                const v = typeof item === 'string' ? item : item.value;
                if (v) labelsMap.get(key)!.add(v);
              });
            }
          }
        }
        setAllLabels(labelsMap);
      }
    } catch (e) { console.error('Error fetching labels:', e); }
  };

  useEffect(() => { fetchWants(); fetchLabels(); }, [fetchWants]);
  usePolling(() => { if (wants.length > 0) fetchWants(); fetchLabels(); }, { interval: 2000, enabled: headerState?.autoRefresh ?? false, immediate: false });

  useEffect(() => {
    if (sidebar.selectedItem) {
      const wantId = sidebar.selectedItem.metadata?.id || sidebar.selectedItem.id;
      if (!wants.some(w => (w.metadata?.id === wantId) || (w.id === wantId))) sidebar.clearSelection();
    }
  }, [wants, sidebar.selectedItem]);

  useEffect(() => { if (error) { const t = setTimeout(() => clearError(), 5000); return () => clearTimeout(t); } }, [error, clearError]);

  const handleToggleSelectMode = () => { if (isSelectMode) { setSelectedWantIds(new Set()); sidebar.closeBatch(); setIsSelectMode(false); } else { setIsSelectMode(true); } };
  const handleSelectWant = (id: string) => {
    setLastSelectedWantId(id);
    setSelectedWantIds(prev => {
      const s = new Set(prev);
      if (s.has(id)) { s.delete(id); if (s.size === 0) sidebar.closeBatch(); } 
      else { s.add(id); if (s.size === 1) sidebar.openBatch(); }
      return s;
    });
  };

  const handleBatchConfirm = async () => {
    if (!batchAction || selectedWantIds.size === 0) return;
    setIsBatchProcessing(true);
    const ids = Array.from(selectedWantIds);
    try {
      if (batchAction === 'start') {
        for (const id of ids) {
          await startWant(id);
        }
        showNotification(`Started ${ids.length} wants`);
      } else if (batchAction === 'stop') {
        for (const id of ids) {
          await stopWant(id);
        }
        showNotification(`Stopped ${ids.length} wants`);
      } else if (batchAction === 'delete') {
        await deleteWants(ids);
        showNotification(`Deleted ${ids.length} wants`);
        setSelectedWantIds(new Set());
        sidebar.closeBatch();
      }
      setShowBatchConfirmation(false); setBatchAction(null);
    } catch (e) { console.error(e); showNotification(`Failed to ${batchAction} some wants`); }
    finally { setIsBatchProcessing(false); }
  };

  const handleBatchCancel = () => { setShowBatchConfirmation(false); setBatchAction(null); };
  const handleDeleteWantCancel = () => { setShowDeleteConfirmation(false); setDeleteWantState(null); };
  const handleDeleteDraftConfirm = async () => {
    if (deleteDraftState) {
      try {
        await apiClient.deleteDraftWant(deleteDraftState.id);
        setShowDeleteDraftConfirmation(false);
        setDeleteDraftState(null);
        showNotification(`Deleted draft`);
        await fetchWants();
      } catch (e) {
        showNotification('Failed to delete draft');
      }
    }
  };
  const handleDeleteDraftCancel = () => { setShowDeleteDraftConfirmation(false); setDeleteDraftState(null); };
  const handleReactionCancel = () => { setShowReactionConfirmation(false); setReactionWantState(null); setReactionAction(null); };
  const handleCreateWant = () => { setEditingWant(null); sidebar.openForm(); };
  const handleEditWant = (w: Want) => { setEditingWant(w); sidebar.openForm(); };

  const handleViewWant = (want: Want | { id: string; parentId?: string }) => {
    const wantToView = 'metadata' in want ? want : wants.find(w => (w.metadata?.id === want.id) || (w.id === want.id));
    if (wantToView) {
      sidebar.selectItem(wantToView);
      setSidebarInitialTab('settings');
      const wantId = wantToView.metadata?.id || wantToView.id;
      if (wantId) setLastSelectedWantId(wantId);
    }
  };

  const handleViewAgents = (want: Want) => { sidebar.selectItem(want); setSidebarInitialTab('agents'); const wantId = want.metadata?.id || want.id; if (wantId) setLastSelectedWantId(wantId); };
  const handleViewResults = (want: Want) => { sidebar.selectItem(want); setSidebarInitialTab('results'); const wantId = want.metadata?.id || want.id; if (wantId) setLastSelectedWantId(wantId); };

  const handleDraftClick = (draft: DraftWant) => {
    setActiveDraftId(draft.id);
    if (draft.recommendations.length > 0) {
      setRecommendations(draft.recommendations);
      setSelectedRecommendation(draft.selectedRecommendation);
      setShowRecommendationForm(true);
      setEditingWant(null);
      sidebar.openForm();
    }
  };

  const handleMinimapClick = (wantId: string) => {
    const element = document.querySelector(`[data-want-id="${wantId}"]`);
    element?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  };

  const handleMinimapDraftClick = (draftId: string) => {
    const element = document.querySelector(`[data-draft-id="${draftId}"]`);
    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }
    // Also activate the draft (same behavior as clicking the draft card)
    const draft = drafts.find(d => d.id === draftId);
    if (draft) handleDraftClick(draft);
  };

  const handleDraftDelete = (draft: DraftWant) => { setDeleteDraftState(draft); setShowDeleteDraftConfirmation(true); };

  const handleInteractSubmit = async (message: string) => {
    let sid: string;
    try { const s = await apiClient.createInteractSession(); sid = s.session_id; } 
    catch (e) { showNotification('Failed to create session'); return; }
    let did: string;
    try { const r = await apiClient.createDraftWant({ sessionId: sid, message, isThinking: true }); did = r.id; } 
    catch (e) { showNotification('Failed to create draft'); return; }
    setActiveDraftId(did); await fetchWants();
    try {
      const r = await apiClient.sendInteractMessage(sid, { message, context: { provider: gooseProvider } });
      if (!r || !Array.isArray(r.recommendations) || r.recommendations.length === 0) {
        await apiClient.updateDraftWant(did, { isThinking: false }); await fetchWants(); return;
      }
      await apiClient.updateDraftWant(did, { recommendations: r.recommendations, isThinking: false });
      await fetchWants(); setRecommendations(r.recommendations); setShowRecommendationForm(true); setEditingWant(null); sidebar.openForm();
    } catch (e: any) {
      showNotification(`Failed: ${e.message}`);
      try { await apiClient.updateDraftWant(did, { error: e.message, isThinking: false }); await fetchWants(); } catch (e2) {}
      setRecommendations([]); setShowRecommendationForm(false);
    }
  };

  const handleRecommendationDeploy = async (rid: string, mods?: ConfigModifications) => {
    const ad = drafts.find(d => d.id === activeDraftId);
    if (!ad) return;
    try {
      const r = await apiClient.deployRecommendation(ad.sessionId, { recommendation_id: rid, modifications: mods });
      showNotification(`Deployed ${r.want_ids.length} want(s) successfully!`);
      if (activeDraftId) { try { await apiClient.deleteDraftWant(activeDraftId); } catch (e) {} setActiveDraftId(null); }
      await fetchWants(); setShowRecommendationForm(false); setRecommendations([]); setSelectedRecommendation(null); sidebar.closeForm();
    } catch (e: any) { showNotification(`Deployment failed: ${e.message}`); }
  };

  const handleLabelClick = async (key: string, value: string) => {
    setSelectedLabel({ key, value });
    setLabelOwners([]);
    setLabelUsers([]);
    
    // Trigger highlight animation on cards
    useWantStore.getState().setHighlightedLabel({ key, value });

    try {
      const r = await fetch('http://localhost:8080/api/v1/labels');
      if (!r.ok) return;
      const d = await r.json();
      if (d.labelValues && d.labelValues[key]) {
        const info = d.labelValues[key].find((i: any) => i.value === value);
        if (info) {
          const wr = await fetch('http://localhost:8080/api/v1/wants');
          if (wr.ok) {
            const wd = await wr.json();
            setLabelOwners(wd.wants.filter((w: Want) => info.owners.includes(w.metadata?.id || w.id || '')));
            setLabelUsers(wd.wants.filter((w: Want) => info.users.includes(w.metadata?.id || w.id || '')));
          }
        }
      }
    } catch (e) {}
  };

  const handleDeleteWantConfirm = async () => {
    if (deleteWantState) {
      try {
        setIsDeletingWant(true);
        const id = deleteWantState.metadata?.id || deleteWantState.id;
        if (!id) return;
        await deleteWant(id);
        setShowDeleteConfirmation(false); setDeleteWantState(null);
        if (selectedWant && (selectedWant.metadata?.id === id || selectedWant.id === id)) sidebar.clearSelection();
      } catch (e) {} finally { setIsDeletingWant(false); }
    }
  };

  const handleShowDeleteConfirmation = (want: Want) => { setDeleteWantState(want); setShowDeleteConfirmation(true); };

  const handleReactionConfirm = async () => {
    if (!reactionWantState || !reactionAction) return;
    setIsSubmittingReaction(true);
    try {
      const qid = reactionWantState.state?.reaction_queue_id as string | undefined;
      if (!qid) return;
      const r = await fetch(`/api/v1/reactions/${qid}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ approved: reactionAction === 'approve', comment: `User ${reactionAction === 'approve' ? 'approved' : 'denied'} reminder` }) });
      if (!r.ok) throw new Error(`Failed: ${r.statusText}`);
      setShowReactionConfirmation(false); setReactionWantState(null); setReactionAction(null);
    } catch (e) {} finally { setIsSubmittingReaction(false); }
  };

  const handleShowReactionConfirmation = (want: Want, action: 'approve' | 'deny') => { setReactionWantState(want); setReactionAction(action); setShowReactionConfirmation(true); };

  const handleSuspendWant = async (want: Want) => {
    const wantId = want.metadata?.id || want.id;
    if (!wantId) return;
    try { await suspendWant(wantId); } catch (e) { console.error('Failed to suspend want:', e); }
  };

  const handleResumeWant = async (want: Want) => {
    const wantId = want.metadata?.id || want.id;
    if (!wantId) return;
    try { await resumeWant(wantId); } catch (e) { console.error('Failed to resume want:', e); }
  };

  const handleSaveRecipeFromWant = async (w: Want) => {
    const id = w.metadata?.id || w.id; if (!id) return;
    try {
      const r = await apiClient.saveRecipeFromWant(id, { name: `${w.metadata.name}-recipe`, description: `Saved from ${w.metadata.name}`, version: '1.0.0' });
      showNotification(`✓ Recipe '${r.id}' saved successfully`);
    } catch (e: any) { showNotification(`✗ Failed: ${e.message}`); }
  };

  const handleUnparentWant = async (id: string) => {
    try {
      const w = wants.find(x => (x.metadata?.id === id) || (x.id === id));
      if (!w || !w.metadata.ownerReferences || w.metadata.ownerReferences.length === 0) return;
      const pids = w.metadata.ownerReferences.filter(r => r.controller && r.kind === 'Want').map(r => r.id).filter((i): i is string => !!i);
      await apiClient.updateWant(id, { ...w, metadata: { ...w.metadata, ownerReferences: [] } });
      showNotification(`✓ Removed parent from ${w.metadata.name}`); await fetchWants();
      setExpandedParents(prev => { const n = new Set(prev); pids.forEach(p => n.delete(p)); return n; });
    } catch (e: any) { showNotification(`✗ Failed: ${e.message}`); }
  };

  const handleTemplateDropped = async (tid: string, tt: 'want-type' | 'recipe') => {
    try {
      const { recipes } = useRecipeStore.getState();
      let params = {}; let wantType = tid;
      if (tt === 'want-type') {
        const wt = await apiClient.getWantType(tid);
        if (wt) {
          if (wt.examples && wt.examples.length > 0) params = wt.examples[0].want?.spec?.params || {};
          else if (wt.parameters) { const d: any = {}; wt.parameters.forEach(p => { if (p.default !== undefined) d[p.name] = p.default; else if (p.example !== undefined) d[p.name] = p.example; }); params = d; }
        }
      } else {
        const r = recipes.find(x => x.recipe?.metadata?.custom_type === tid);
        if (r) { params = r.recipe.parameters || {}; wantType = r.recipe.metadata.custom_type || tid; }
      }
      const name = generateUniqueWantName(tid, tt, new Set(wants.map(w => w.metadata?.name || '')));
      const { createWant } = useWantStore.getState();
      await createWant({ metadata: { name, type: wantType, labels: { 'mywant.io/type': wantType } }, spec: { params } });
      setDraggingTemplate(null); showNotification(`✓ Created "${name}"`); await fetchWants();
    } catch (e: any) { showNotification(`✗ Failed: ${e.message}`); }
  };

  const handleLabelDropped = async (wantId: string) => {
    await fetchWants();
    const want = wants.find(w => (w.metadata?.id === wantId) || (w.id === wantId));
    if (want) { sidebar.selectItem(want); setSidebarInitialTab('settings'); }
  };

  const handleWantDropped = async (draggedWantId: string, targetWantId: string) => {
    try {
      const draggedWant = wants.find(w => (w.metadata?.id === draggedWantId) || (w.id === draggedWantId));
      if (!draggedWant) return;
      if (!targetWantId) { if (draggedWant.metadata.ownerReferences?.length) await handleUnparentWant(draggedWantId); return; }
      const targetWant = wants.find(w => (w.metadata?.id === targetWantId) || (w.id === targetWantId));
      if (!targetWant) return;
      const ownerRef = { apiVersion: 'mywant/v1', kind: 'Want', name: targetWant.metadata.name, id: targetWantId, controller: true, blockOwnerDeletion: true };
      await apiClient.updateWant(draggedWantId, { ...draggedWant, metadata: { ...draggedWant.metadata, ownerReferences: [...(draggedWant.metadata.ownerReferences || []), ownerRef] } });
      showNotification(`✓ Added ${draggedWant.metadata.name} to ${targetWant.metadata.name}`);
      await fetchWants(); handleToggleExpand(targetWantId);
    } catch (error) { showNotification(`✗ Failed: ${error instanceof Error ? error.message : 'Unknown error'}`); }
  };

  const handleGlobalDragEnter = (e: React.DragEvent) => {
    const isTemplate = draggingTemplate || e.dataTransfer.types.includes('application/mywant-template');
    if (isTemplate) {
      e.preventDefault();
      setDragCounter(prev => {
        const next = prev + 1;
        if (next === 1) setIsGlobalDragOver(true);
        return next;
      });
    }
  };

  const handleGlobalDragOver = (e: React.DragEvent) => {
    const isTemplate = draggingTemplate || e.dataTransfer.types.includes('application/mywant-template');
    const isWant = e.dataTransfer.types.includes('application/mywant-id');
    if (isTemplate || isWant) {
      e.preventDefault();
      e.dataTransfer.dropEffect = isTemplate ? 'copy' : 'move';
    }
  };

  const handleGlobalDragLeave = (e: React.DragEvent) => {
    const isTemplate = draggingTemplate || e.dataTransfer.types.includes('application/mywant-template');
    if (isTemplate) {
      setDragCounter(prev => {
        const next = Math.max(0, prev - 1);
        if (next === 0) setIsGlobalDragOver(false);
        return next;
      });
    }
  };

  const handleGlobalDrop = (e: React.DragEvent) => {
    const templateData = e.dataTransfer.getData('application/mywant-template');
    const draggedWantId = e.dataTransfer.getData('application/mywant-id');
    
    if (templateData || draggedWantId) {
      e.preventDefault();
      setIsGlobalDragOver(false);
      setDragCounter(0);

      if (templateData) {
        try {
          const t = JSON.parse(templateData);
          if (t.id && t.type) handleTemplateDropped(t.id, t.type);
        } catch (err) {}
      } else if (draggedWantId) {
        handleUnparentWant(draggedWantId);
      }
    }
  };

  const handleToggleExpand = (wantId: string) => setExpandedParents(prev => { const next = new Set(prev); if (next.has(wantId)) next.delete(wantId); else next.add(wantId); return next; });
  const handleCloseModals = () => { sidebar.closeForm(); setEditingWant(null); setDeleteWantState(null); setShowDeleteConfirmation(false); setReactionWantState(null); setShowReactionConfirmation(false); setReactionAction(null); };
  
  const handleExportWants = async () => {
    setIsExporting(true);
    try {
      const response = await fetch('http://localhost:8080/api/v1/wants/export', { method: 'POST', headers: { 'Content-Type': 'application/json' } });
      if (!response.ok) throw new Error(`Export failed: ${response.statusText}`);
      const contentDisposition = response.headers.get('Content-Disposition');
      let filename = 'wants-export.yaml';
      if (contentDisposition) {
        const match = contentDisposition.match(/filename="?([^";\n]+)"?/);
        if (match) filename = match[1];
      }
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url; link.download = filename;
      document.body.appendChild(link); link.click(); document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
      showNotification('✓ Exported wants to YAML');
    } catch (error) { showNotification(`✗ Export failed: ${error instanceof Error ? error.message : 'Unknown error'}`); }
    finally { setIsExporting(false); }
  };

  const handleImportWants = async (file: File) => {
    setIsImporting(true);
    try {
      const fileText = await file.text();
      const response = await fetch('http://localhost:8080/api/v1/wants/import', { method: 'POST', headers: { 'Content-Type': 'application/yaml' }, body: fileText });
      if (!response.ok) { const errorData = await response.text(); throw new Error(`Import failed: ${errorData || response.statusText}`); }
      const result = await response.json();
      showNotification(`✓ Imported ${result.wants} want(s)`);
      fetchWants();
      if (fileInputRef.current) fileInputRef.current.value = '';
    } catch (error) { showNotification(`✗ Import failed: ${error instanceof Error ? error.message : 'Unknown error'}`); }
    finally { setIsImporting(false); }
  };

  const handleFileInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file && (file.name.endsWith('.yaml') || file.name.endsWith('.yml'))) handleImportWants(file);
  };

  // ============================================================
  // Dashboard Modes:
  // - Normal Mode: Default mode with all shortcuts enabled
  // - Select Mode: Shift+S to enter, batch operations enabled (isSelectMode=true)
  // - Add Mode: New Want form open, all dashboard shortcuts disabled (sidebar.showForm=true)
  // - Summary Mode: Summary panel open, all shortcuts enabled (sidebar.showSummary=true)
  // ============================================================

  useHierarchicalKeyboardNavigation({ items: hierarchicalWants, currentItem: currentHierarchicalWant, onNavigate: handleViewWant, onToggleExpand: handleToggleExpand, onSelect: isSelectMode ? handleSelectWant : undefined, expandedItems: expandedParents, lastSelectedItemId: lastSelectedWantId, enabled: !sidebar.showForm && filteredWants.length > 0 });

  const handleEscapeKey = () => {
    // Otherwise, handle other escape scenarios
    if (showBatchConfirmation) setShowBatchConfirmation(false);
    else if (selectedWant) { setLastSelectedWantId(selectedWant.metadata?.id || selectedWant.id || null); sidebar.clearSelection(); }
    else if (sidebar.showSummary) sidebar.closeSummary();
    else if (sidebar.showForm) sidebar.closeForm();
    else if (isSelectMode) { sidebar.closeBatch(); setSelectedWantIds(new Set()); setIsSelectMode(false); }
  };
  useEscapeKey({ onEscape: handleEscapeKey, enabled: !!selectedWant || sidebar.showSummary || sidebar.showForm || isSelectMode });

  // Separate Escape handler for interact input (since useEscapeKey ignores input elements)
  // Use capture phase to catch the event before other handlers
  useEffect(() => {
    const handleInteractInputEscape = (e: KeyboardEvent) => {
      console.log('[Dashboard] Escape handler triggered, key:', e.key, 'activeElement:', document.activeElement);
      if (e.key === 'Escape') {
        const interactInput = document.querySelector('[data-interact-input]') as HTMLInputElement;
        console.log('[Dashboard] interactInput element:', interactInput, 'matches activeElement:', interactInput === document.activeElement);
        if (interactInput && document.activeElement === interactInput) {
          e.preventDefault();
          e.stopPropagation();
          console.log('[Dashboard] Escape pressed on interact input, blurring');
          interactInput.blur();
        }
      }
    };

    // Use capture phase (true) to catch event before it reaches other handlers
    window.addEventListener('keydown', handleInteractInputEscape, true);
    return () => window.removeEventListener('keydown', handleInteractInputEscape, true);
  }, []);

  // Keyboard shortcuts: a (add), s (summary), Shift+S (select), Ctrl+A (select all in select mode), q (focus suggestion input)
  // NOTE: All dashboard shortcuts are DISABLED when in Add Mode (sidebar.showForm=true)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // CRITICAL: Disable all dashboard shortcuts when New Want form is open (Add Mode)
      if (sidebar.showForm) {
        return;
      }

      // Don't intercept if user is typing in an input
      const target = e.target as HTMLElement;
      const isInputElement =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      if (isInputElement) return;

      // Handle shortcuts
      // IMPORTANT: Check Ctrl+A / Cmd+A FIRST before simple 'a' to prevent conflicts
      if (e.key === 'a' && (e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey) {
        // Ctrl+A (or Cmd+A on Mac) - Select all wants in select mode
        // Must preventDefault BEFORE checking isSelectMode to block browser default
        e.preventDefault();
        e.stopPropagation();
        if (isSelectMode) {
          const allWantIds = new Set(filteredWants.map(w => w.metadata?.id || w.id || '').filter(id => id !== ''));
          setSelectedWantIds(allWantIds);
          if (allWantIds.size > 0) {
            sidebar.openBatch();
          }
        }
      } else if (e.key === 'a' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        handleCreateWant();
      } else if (e.key === 's' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        sidebar.toggleSummary();
      } else if (e.key === 'S' && e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        handleToggleSelectMode();
      } else if (e.key === 'q' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        console.log('[Dashboard] q key pressed, attempting to focus interact input');
        // Focus interact input (Suggestion textbox)
        // Use requestAnimationFrame to ensure DOM is ready and visible
        requestAnimationFrame(() => {
          const interactInput = document.querySelector('[data-interact-input]') as HTMLInputElement;
          console.log('[Dashboard] interactInput element:', interactInput);
          if (interactInput) {
            // Check if element is visible
            const isVisible = interactInput.offsetParent !== null;
            console.log('[Dashboard] interactInput isVisible:', isVisible);
            if (isVisible) {
              interactInput.focus();
              console.log('[Dashboard] Focus set, activeElement:', document.activeElement);
              // Scroll into view if needed
              interactInput.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
            } else {
              console.warn('[Dashboard] Interact input exists but is not visible (may be hidden on mobile)');
            }
          } else {
            console.warn('[Dashboard] Interact input not found in DOM');
          }
        });
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleCreateWant, handleToggleSelectMode, sidebar, sidebar.showForm, isSelectMode, filteredWants]);

  const getWantBackgroundImage = (type?: string) => {
    if (!type) return undefined;
    const lowerType = type.toLowerCase();
    if (lowerType === 'flight') return '/resources/flight.png';
    if (lowerType === 'hotel') return '/resources/hotel.png';
    if (lowerType === 'restaurant') return '/resources/restaurant.png';
    if (lowerType === 'buffet') return '/resources/buffet.png';
    if (lowerType === 'evidence') return '/resources/evidence.png';
    if (lowerType.includes('coordinator')) return '/resources/agent.png';
    if (lowerType.includes('prime') || lowerType.includes('fibonacci') || lowerType.includes('numbers')) return '/resources/numbers.png';
    if (lowerType === 'scheduler' || lowerType.includes('execution')) return '/resources/screen.png';
    return undefined;
  };

  const wantBackgroundImage = getWantBackgroundImage(selectedWant?.metadata?.type);
  const headerActions = headerState ? (
    <div className="flex items-center gap-2">
      <StatusBadge status={headerState.status} size="sm" />
      <button onClick={() => sidebar.toggleHeaderAction?.('autoRefresh')} className={`p-2 rounded-md transition-colors ${headerState.autoRefresh ? 'bg-blue-100 text-blue-600 hover:bg-blue-200' : 'text-gray-400 hover:text-gray-600 hover:bg-gray-100'}`} title={headerState.autoRefresh ? 'Disable auto refresh' : 'Enable auto refresh'}><RefreshCw className="h-4 w-4" /></button>
      <button onClick={() => sidebar.toggleHeaderAction?.('refresh')} disabled={headerState.loading} className="p-2 text-gray-400 hover:text-gray-600 hover:bg-gray-100 rounded-md transition-colors disabled:opacity-50" title="Refresh"><RefreshCw className={classNames('h-4 w-4', headerState.loading && 'animate-spin')} /></button>
    </div>
  ) : null;

  return (
    <Layout sidebarMinimized={sidebarMinimized} onSidebarMinimizedChange={setSidebarMinimized}>
      <Header
        onCreateWant={handleCreateWant}
        showSummary={sidebar.showSummary}
        onSummaryToggle={sidebar.toggleSummary}
        sidebarMinimized={sidebarMinimized}
        showSelectMode={isSelectMode}
        onToggleSelectMode={handleToggleSelectMode}
        onInteractSubmit={handleInteractSubmit}
        isInteractThinking={hasThinkingDraft}
        gooseProvider={gooseProvider}
        onProviderChange={setGooseProvider}
        showMinimap={minimapOpen}
        onMinimapToggle={() => setMinimapOpen(!minimapOpen)}
      />
      <main
        className="flex-1 flex overflow-hidden bg-gray-50 mt-16 lg:mr-[480px] mr-0 relative"
        onDragEnter={handleGlobalDragEnter}
        onDragOver={handleGlobalDragOver}
        onDragLeave={handleGlobalDragLeave}
        onDrop={handleGlobalDrop}
      >
        {/* Confirmation Notification - Dashboard Right Layout */}
        {(showDeleteConfirmation || showReactionConfirmation || showBatchConfirmation || showDeleteDraftConfirmation) && (
          <ConfirmationBubble
            message={
              showDeleteConfirmation
                ? (deleteWantState ? `Delete: ${deleteWantState.metadata?.name || deleteWantState.metadata?.id || deleteWantState.id}` : null)
                : showDeleteDraftConfirmation
                ? (deleteDraftState ? `Delete draft: ${deleteDraftState.message}` : null)
                : showBatchConfirmation
                ? `${batchAction === 'delete' ? 'Delete' : batchAction === 'start' ? 'Start' : 'Stop'} ${selectedWantIds.size} wants?`
                : (reactionWantState ? `${reactionAction === 'approve' ? 'Approve' : 'Deny'}: ${reactionWantState.metadata?.name || reactionWantState.metadata?.id || reactionWantState.id}` : null)
            }
            isVisible={showDeleteConfirmation || showReactionConfirmation || showBatchConfirmation || showDeleteDraftConfirmation}
            onDismiss={() => {
              setShowDeleteConfirmation(false);
              setShowReactionConfirmation(false);
              setShowBatchConfirmation(false);
              setShowDeleteDraftConfirmation(false);
            }}
            onConfirm={
              showDeleteConfirmation
                ? handleDeleteWantConfirm
                : showDeleteDraftConfirmation
                ? handleDeleteDraftConfirm
                : showBatchConfirmation
                ? handleBatchConfirm
                : handleReactionConfirm
            }
            onCancel={
              showDeleteConfirmation
                ? handleDeleteWantCancel
                : showDeleteDraftConfirmation
                ? handleDeleteDraftCancel
                : showBatchConfirmation
                ? handleBatchCancel
                : handleReactionCancel
            }
            loading={isDeletingWant || isSubmittingReaction || isBatchProcessing}
            title={showDeleteConfirmation ? "Delete Want" : showDeleteDraftConfirmation ? "Delete Draft" : showBatchConfirmation ? `Batch ${batchAction}` : "Confirm"}
            layout="dashboard-right"
          />
        )}
        <div className={classNames("flex-1 overflow-y-auto transition-colors duration-200", isGlobalDragOver && "bg-blue-50 border-4 border-dashed border-blue-400 border-inset")}>
          <div className="p-6 pb-24 flex flex-col h-full min-h-screen">
            <React.Fragment>
              {error && <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-md flex items-center"><div className="ml-3"><p className="text-sm text-red-700">{error}</p></div><button onClick={clearError} className="ml-auto text-red-400 hover:text-red-600"><svg className="h-4 w-4" viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" /></svg></button></div>}
              <div className="flex-1 flex flex-col">
                <WantGrid
                  wants={regularWants} drafts={drafts} activeDraftId={activeDraftId} onDraftClick={handleDraftClick} onDraftDelete={handleDraftDelete} loading={loading} searchQuery={searchQuery} statusFilters={statusFilters} selectedWant={selectedWant} onViewWant={handleViewWant} onViewAgentsWant={handleViewAgents} onViewResultsWant={handleViewResults} onEditWant={handleEditWant} onDeleteWant={handleShowDeleteConfirmation} onSuspendWant={handleSuspendWant} onResumeWant={handleResumeWant} onGetFilteredWants={setFilteredWants} expandedParents={expandedParents} onToggleExpand={handleToggleExpand} onCreateWant={handleCreateWant} onLabelDropped={handleLabelDropped} onWantDropped={handleWantDropped} onShowReactionConfirmation={handleShowReactionConfirmation} isSelectMode={isSelectMode} selectedWantIds={selectedWantIds} onSelectWant={handleSelectWant}
                />
              </div>
            </React.Fragment>
          </div>
        </div>
      </main>
      <RightSidebar 
        isOpen={sidebar.showSummary || !!selectedWant || sidebar.showBatch} 
        onClose={() => { 
          if (sidebar.showSummary) sidebar.closeSummary();
          else if (isSelectMode) { sidebar.closeBatch(); setSelectedWantIds(new Set()); } 
          else { sidebar.clearSelection(); } 
        }} 
        title={isSelectMode ? 'Batch Actions' : (selectedWant ? (selectedWant.metadata?.name || selectedWant.metadata?.id || 'Want Details') : 'Summary')} 
        backgroundStyle={!isSelectMode && selectedWant ? { backgroundImage: `url(${wantBackgroundImage})`, backgroundSize: 'cover', backgroundPosition: 'center', backgroundAttachment: 'fixed' } : undefined} 
        headerActions={!isSelectMode && selectedWant ? headerActions : undefined}
      >
        {isSelectMode ? (
          <WantBatchControlPanel 
            selectedCount={selectedWantIds.size} 
            onBatchStart={() => { setBatchAction('start'); setShowBatchConfirmation(true); }} 
            onBatchStop={() => { setBatchAction('stop'); setShowBatchConfirmation(true); }} 
            onBatchDelete={() => { setBatchAction('delete'); setShowBatchConfirmation(true); }} 
            onBatchCancel={handleToggleSelectMode} 
            loading={isBatchProcessing} 
          />
        ) : (
          <WantDetailsSidebar 
            want={selectedWant} 
            initialTab={sidebarInitialTab} 
            onWantUpdate={() => { if (selectedWant?.metadata?.id || selectedWant?.id) useWantStore.getState().fetchWantDetails((selectedWant.metadata?.id || selectedWant.id) as string); }} 
            onHeaderStateChange={setHeaderState} 
            onRegisterHeaderActions={sidebar.registerHeaderActions} 
            onStart={() => startWant(selectedWant?.metadata?.id || selectedWant?.id || '')} 
            onStop={() => stopWant(selectedWant?.metadata?.id || selectedWant?.id || '')} 
            onSuspend={() => suspendWant(selectedWant?.metadata?.id || selectedWant?.id || '')} 
            onResume={() => resumeWant(selectedWant?.metadata?.id || selectedWant?.id || '')} 
            onDelete={() => handleShowDeleteConfirmation(selectedWant!)} 
            onSaveRecipe={() => handleSaveRecipeFromWant(selectedWant!)} 
            summaryProps={{
              wants,
              loading,
              searchQuery,
              onSearchChange: setSearchQuery,
              statusFilters,
              onStatusFilter: setStatusFilters,
              allLabels,
              onLabelClick: handleLabelClick,
              selectedLabel,
              onClearSelectedLabel: () => { setSelectedLabel(null); setLabelOwners([]); setLabelUsers([]); },
              labelOwners,
              labelUsers,
              onViewWant: handleViewWant,
              onExportWants: handleExportWants,
              onImportWants: () => fileInputRef.current?.click(),
              isExporting,
              isImporting,
              fetchLabels,
              fetchWants
            }}
          />
        )}
      </RightSidebar>
      <WantMinimap
        wants={filteredWants}
        drafts={drafts}
        selectedWantId={selectedWant?.metadata?.id || selectedWant?.id}
        activeDraftId={activeDraftId}
        onWantClick={handleMinimapClick}
        onDraftClick={handleMinimapDraftClick}
        isOpen={minimapOpen}
      />
      <WantForm isOpen={sidebar.showForm} onClose={handleCloseModals} editingWant={editingWant} mode={showRecommendationForm ? 'recommendation' : (editingWant ? 'edit' : 'create')} recommendations={recommendations} selectedRecommendation={selectedRecommendation} onRecommendationSelect={setSelectedRecommendation} onRecommendationDeploy={handleRecommendationDeploy} />

      <Toast message={notificationMessage} isVisible={isNotificationVisible} onDismiss={dismissNotification} />
      <DragOverlay />
    </Layout>
  );
};

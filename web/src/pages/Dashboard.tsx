import React, { useState, useEffect, useRef, useMemo } from 'react';
import { RefreshCw, ChevronDown, Heart, StickyNote } from 'lucide-react';
import { WantExecutionStatus, Want } from '@/types/want';
import { useWantStore } from '@/stores/wantStore';
import { useWantTypeStore } from '@/stores/wantTypeStore';
import { useRecipeStore } from '@/stores/recipeStore';
import { useUIStore } from '@/stores/uiStore';
import { usePolling } from '@/hooks/usePolling';
import { smartPollWants, seedWantETags } from '@/stores/wantHashCache';
import { POLLING_INTERVAL_MS } from '@/constants/polling';
import { useHierarchicalKeyboardNavigation } from '@/hooks/useHierarchicalKeyboardNavigation';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { useRightSidebarExclusivity } from '@/hooks/useRightSidebarExclusivity';
import { StatusBadge } from '@/components/common/StatusBadge';
import { classNames, truncateText } from '@/utils/helpers';
import { getBackgroundImage } from '@/utils/backgroundStyles';
import { addLabelToRegistry } from '@/utils/labelUtils';
import { generateUniqueWantName } from '@/utils/nameGenerator';
import { apiClient } from '@/api/client';
import { Recommendation, ConfigModifications } from '@/types/interact';
import { DraftWant, wantToDraft, isDraftWant } from '@/types/draft';
import { WantRecipeAnalysis, RecipeMetadata, StateDef } from '@/types/recipe';

// Components
import { Header } from '@/components/layout/Header';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { WantGrid } from '@/components/dashboard/WantGrid';
import { WantMinimap } from '@/components/dashboard/WantMinimap';
import { WantForm } from '@/components/forms/WantForm';
import { WantDetailsSidebar } from '@/components/sidebar/WantDetailsSidebar';
import { GlobalStateSidebar } from '@/components/sidebar/GlobalStateSidebar';
import { SummarySidebarContent } from '@/components/sidebar/SummarySidebarContent';
import { BatchActionBar } from '@/components/dashboard/BatchActionBar';
import { HeaderOverlay } from '@/components/layout/HeaderOverlay';
import { Toast } from '@/components/notifications';
import { DragOverlay } from '@/components/dashboard/DragOverlay';
import { SaveAsRecipeModal } from '@/components/modals/SaveAsRecipeModal';


export const Dashboard: React.FC = () => {
  const { 
    wants, loading, error, fetchWants, deleteWant, deleteWants, 
    suspendWant, resumeWant, stopWant, startWant, clearError, 
    draggingTemplate, setDraggingTemplate 
  } = useWantStore();

  const sidebar = useRightSidebarExclusivity<Want>();
  const [expandedChain, setExpandedChain] = useState<Want[]>([]);
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
  const [sidebarInitialTab, setSidebarInitialTab] = useState<'settings' | 'results' | 'logs' | 'agents' | 'chat'>('results');
  const [sidebarTabVersion, setSidebarTabVersion] = useState(0);
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
  const cardListScrollRef = useRef<HTMLDivElement>(null);
  const [notificationMessage, setNotificationMessage] = useState<string | null>(null);
  const [isNotificationVisible, setIsNotificationVisible] = useState(false);
  const [activeDraftId, setActiveDraftId] = useState<string | null>(null);
  const [deleteDraftState, setDeleteDraftState] = useState<DraftWant | null>(null);
  const [showDeleteDraftConfirmation, setShowDeleteDraftConfirmation] = useState(false);
  const [showSaveRecipeModal, setShowSaveRecipeModal] = useState(false);
  const [saveRecipeTarget, setSaveRecipeTarget] = useState<Want | null>(null);
  const [saveRecipeAnalysis, setSaveRecipeAnalysis] = useState<WantRecipeAnalysis | null>(null);
  const [saveRecipeLoading, setSaveRecipeLoading] = useState(false);

  // Global Drag and Drop state
  const [isGlobalDragOver, setIsGlobalDragOver] = useState(false);
  const [dragCounter, setDragCounter] = useState(0);

  // Minimap state
  const [minimapOpen, setMinimapOpen] = useState(window.innerWidth >= 1024); // Desktop default: true, Mobile: false
  const [radarMode, setRadarMode] = useState(false);
  const [showGlobalState, setShowGlobalState] = useState(false);

  const drafts = useMemo(() => wants.filter(isDraftWant).map(wantToDraft).filter((d): d is DraftWant => d !== null), [wants]);
  const regularWants = useMemo(() => wants.filter(w => !isDraftWant(w)), [wants]);
  const [recommendations, setRecommendations] = useState<Recommendation[]>([]);
  const [selectedRecommendation, setSelectedRecommendation] = useState<Recommendation | null>(null);
  const [showRecommendationForm, setShowRecommendationForm] = useState(false);
  const [gooseProvider, setGooseProvider] = useState<string>('claude-code');
  const hasThinkingDraft = drafts.some(d => d.isThinking);
  const [isInteractSubmitting, setIsInteractSubmitting] = useState(false);

  const showNotification = (message: string) => { setNotificationMessage(message); setIsNotificationVisible(true); };
  const dismissNotification = () => { setIsNotificationVisible(false); setNotificationMessage(null); };

  const selectedWant = useMemo(() => {
    if (!sidebar.selectedItem) return null;
    const wantId = sidebar.selectedItem.metadata?.id || sidebar.selectedItem.id;
    return wants.find(w => (w.metadata?.id === wantId) || (w.id === wantId)) || sidebar.selectedItem;
  }, [sidebar.selectedItem, wants]);

  const [seriesWants, setSeriesWants] = useState<Want[]>([]);
  useEffect(() => {
    const series = selectedWant?.metadata?.series;
    if (!series) { setSeriesWants([]); return; }
    apiClient.listWants({ includeCancelled: true })
      .then(all => setSeriesWants(all.filter(w => w.metadata?.series === series)))
      .catch(() => setSeriesWants([]));
  }, [selectedWant?.metadata?.series]);

  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilters, setStatusFilters] = useState<WantExecutionStatus[]>([]);
  const [filteredWants, setFilteredWants] = useState<Want[]>([]);
  const flattenedWants = filteredWants.flatMap((pw: any) => [pw, ...(pw.children || [])]);
  const hierarchicalWants = flattenedWants.map(w => ({ id: w.metadata?.id || w.id || '', parentId: w.metadata?.ownerReferences?.[0]?.id }));
  const currentHierarchicalWant = selectedWant ? { id: selectedWant.metadata?.id || selectedWant.id || '', parentId: selectedWant.metadata?.ownerReferences?.[0]?.id } : null;

  // Map of wantID -> rate for correlation highlighting (only populated when radarMode is active and a want is selected).
  // Prefer the polled `selectedWant` (has latest correlation) but fall back to sidebar.selectedItem.
  const correlationHighlights = useMemo<Map<string, number>>(() => {
    if (!radarMode) return new Map();
    const source = selectedWant ?? sidebar.selectedItem;
    if (!source) return new Map();
    const entries = source.metadata?.correlation;
    if (!entries?.length) return new Map();
    const map = new Map<string, number>();
    for (const entry of entries) {
      map.set(entry.wantID, entry.rate);
    }
    return map;
  }, [radarMode, selectedWant, sidebar.selectedItem]);
  const [headerState, setHeaderState] = useState<{ autoRefresh: boolean; loading: boolean; status: WantExecutionStatus } | null>(null);

  const fetchLabels = async () => {
    try {
      const response = await fetch('/api/v1/labels');
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

  useEffect(() => {
    fetchWants().then(() => {
      // Seed the ETag cache from the initial full load so that subsequent
      // smart-polling calls can skip unchanged wants via If-None-Match.
      seedWantETags(useWantStore.getState().wants);
    });
    fetchLabels();
  }, [fetchWants]);

  // Smart polling: lightweight hash check → partial fetch of changed wants only
  usePolling(
    () => { if (wants.length > 0) smartPollWants(); fetchLabels(); },
    { interval: POLLING_INTERVAL_MS, enabled: headerState?.autoRefresh ?? false, immediate: false }
  );

  useEffect(() => {
    if (sidebar.selectedItem) {
      const wantId = sidebar.selectedItem.metadata?.id || sidebar.selectedItem.id;
      if (!wants.some(w => (w.metadata?.id === wantId) || (w.id === wantId))) sidebar.clearSelection();
      else setShowGlobalState(false); // Close global state panel when a want is selected
    }
  }, [wants, sidebar.selectedItem]);

  useEffect(() => { if (error) { const t = setTimeout(() => clearError(), 5000); return () => clearTimeout(t); } }, [error, clearError]);

  const handleToggleSelectMode = () => { if (isSelectMode) { setSelectedWantIds(new Set()); setIsSelectMode(false); } else { setIsSelectMode(true); } };
  const handleSelectWant = (id: string) => {
    setLastSelectedWantId(id);
    setSelectedWantIds(prev => {
      const s = new Set(prev);
      if (s.has(id)) { s.delete(id); }
      else { s.add(id); }
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
      }
      setShowBatchConfirmation(false); setBatchAction(null);
      setSelectedWantIds(new Set()); setIsSelectMode(false);
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
  const [ownerWant, setOwnerWant] = useState<Want | null>(null);
  const [initialFormTypeId, setInitialFormTypeId] = useState<string | undefined>(undefined);
  const [initialFormItemType, setInitialFormItemType] = useState<'want-type' | 'recipe'>('want-type');
  const handleCreateWant = (parentWant?: Want) => { setInitialFormTypeId(undefined); setInitialFormItemType('want-type'); setOwnerWant(parentWant || null); setEditingWant(null); sidebar.openForm(); };
  const handleCreateTargetWant = () => { setInitialFormTypeId('whim-target'); setInitialFormItemType('recipe'); setOwnerWant(null); setEditingWant(null); sidebar.openForm(); };
  const handleEditWant = (w: Want) => { setEditingWant(w); sidebar.openForm(); };

  // Walk up the DOM to find the nearest ancestor that actually scrolls.
  const findScrollableAncestor = (el: Element): Element => {
    let node: Element | null = el.parentElement;
    while (node && node !== document.documentElement) {
      const { overflowY } = window.getComputedStyle(node);
      if ((overflowY === 'auto' || overflowY === 'scroll') && node.scrollHeight > node.clientHeight) {
        return node;
      }
      node = node.parentElement;
    }
    return document.documentElement;
  };

  // On mobile (<640px), the bottom sheet covers 70vh — scroll the tapped card to
  // the center of the remaining visible 30% at the top.
  const scrollCardIntoMobileView = (wantId: string) => {
    if (window.innerWidth >= 640) return;
    // setTimeout lets React flush state updates and the iOS touch cycle settle
    // before we measure element positions.
    setTimeout(() => {
      const element = document.querySelector(`[data-want-id="${wantId}"]`);
      if (!element) return;

      const scroller = findScrollableAncestor(element);
      const isDocRoot = scroller === document.documentElement;
      const cardRect = element.getBoundingClientRect();
      // scrollerTop: viewport-y of the scroller's top edge (0 for the document root)
      const scrollerTop = isDocRoot ? 0 : scroller.getBoundingClientRect().top;
      // Visible area: from scrollerTop to 30% of viewport height (sheet covers bottom 70%)
      const visibleAreaCenter = (scrollerTop + window.innerHeight * 0.30) / 2;
      const cardCenterY = cardRect.top + cardRect.height / 2;
      const delta = cardCenterY - visibleAreaCenter;
      const currentScroll = isDocRoot ? window.scrollY : scroller.scrollTop;
      const newScroll = Math.max(0, currentScroll + delta);

      if (isDocRoot) {
        window.scrollTo({ top: newScroll, behavior: 'smooth' });
      } else {
        // Direct assignment is more reliable than scrollTo on iOS
        scroller.scrollTop = newScroll;
      }
    }, 80);
  };

  const handleViewWant = (want: Want | { id: string; parentId?: string }) => {
    const wantToView = 'metadata' in want ? want : wants.find(w => (w.metadata?.id === want.id) || (w.id === want.id));
    if (wantToView) {
      sidebar.selectItem(wantToView);
      setSidebarInitialTab('results');
      const wantId = wantToView.metadata?.id || wantToView.id;
      if (wantId) {
        setLastSelectedWantId(wantId);
        scrollCardIntoMobileView(wantId);
      }
      // Set expanded chain: expand bubble if this want has children
      const hasChildren = wants.some(w =>
        w.metadata?.ownerReferences?.some(ref => ref.id === (wantToView.metadata?.id || wantToView.id))
      );
      if (hasChildren) {
        setExpandedChain([wantToView]);
      } else {
        setExpandedChain([]);
      }
    }
  };

  // Called from WantChildrenBubble when a child want is clicked
  const handleBubbleChildClick = (want: Want) => {
    sidebar.selectItem(want);
    setSidebarInitialTab('results');
    const wantId = want.metadata?.id || want.id;
    if (wantId) setLastSelectedWantId(wantId);
    // Check if this child has children and extend/trim chain accordingly
    const hasChildren = wants.some(w =>
      w.metadata?.ownerReferences?.some(ref => ref.id === wantId)
    );
    if (hasChildren) {
      // Append to expandedChain if not already at the end
      setExpandedChain(prev => {
        const existingIdx = prev.findIndex(w => (w.metadata?.id || w.id) === wantId);
        if (existingIdx !== -1) {
          // Already in chain — trim to this point (collapse deeper)
          return prev.slice(0, existingIdx + 1);
        }
        return [...prev, want];
      });
    }
    // If no children, keep the chain as-is (parent bubble stays open)
  };

  const handleViewAgents = (want: Want) => { sidebar.selectItem(want); setSidebarInitialTab('agents'); const wantId = want.metadata?.id || want.id; if (wantId) { setLastSelectedWantId(wantId); scrollCardIntoMobileView(wantId); } };
  const handleViewResults = (want: Want) => { sidebar.selectItem(want); setSidebarInitialTab('results'); setSidebarTabVersion(v => v + 1); const wantId = want.metadata?.id || want.id; if (wantId) { setLastSelectedWantId(wantId); scrollCardIntoMobileView(wantId); } };
  const handleViewChat = (want: Want) => { sidebar.selectItem(want); setSidebarInitialTab('chat'); const wantId = want.metadata?.id || want.id; if (wantId) { setLastSelectedWantId(wantId); scrollCardIntoMobileView(wantId); } };

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
    useWantStore.getState().setBlinkingWantId(wantId);
    
    // Close minimap on mobile after selection
    if (window.innerWidth < 1024) {
      setMinimapOpen(false);
    }
  };

  const handleMinimapDoubleClick = (wantId: string) => {
    handleMinimapClick(wantId);
    const want = wants.find(w => (w.metadata?.id === wantId) || (w.id === wantId));
    if (want) handleViewWant(want);
  };

  const handleMinimapDraftClick = (draftId: string) => {
    const element = document.querySelector(`[data-draft-id="${draftId}"]`);
    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }
    // Also activate the draft (same behavior as clicking the draft card)
    const draft = drafts.find(d => d.id === draftId);
    if (draft) handleDraftClick(draft);

    // Close minimap on mobile after selection
    if (window.innerWidth < 1024) {
      setMinimapOpen(false);
    }
  };

  const handleDraftDelete = (draft: DraftWant) => { setDeleteDraftState(draft); setShowDeleteDraftConfirmation(true); };

  const handleInteractSubmit = async (message: string) => {
    setIsInteractSubmitting(true);
    try {
      await apiClient.createWant({
        metadata: { name: `whim-${Date.now()}`, type: 'whim-target', labels: { category: 'whim' } },
        spec: { recipe: 'yaml/recipes/whim.yaml', params: { want: message } }
      });
      await fetchWants();
    } catch (e: any) {
      showNotification(`Failed: ${e.message}`);
    } finally {
      setIsInteractSubmitting(false);
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
      const r = await fetch('/api/v1/labels');
      if (!r.ok) return;
      const d = await r.json();
      if (d.labelValues && d.labelValues[key]) {
        const info = d.labelValues[key].find((i: any) => i.value === value);
        if (info) {
          const wr = await fetch('/api/v1/wants');
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

  const handleDirectDeleteWant = async (want: Want) => {
    try {
      setIsDeletingWant(true);
      const id = want.metadata?.id || want.id;
      if (!id) return;
      await deleteWant(id);
      if (selectedWant && (selectedWant.metadata?.id === id || selectedWant.id === id)) sidebar.clearSelection();
    } catch (e) {} finally { setIsDeletingWant(false); }
  };

  const handleReactionConfirm = async () => {
    if (!reactionWantState || !reactionAction) return;
    setIsSubmittingReaction(true);
    try {
      const qid = reactionWantState.state?.current?.reaction_queue_id as string | undefined;
      if (!qid) return;
      const isGoal = reactionWantState.metadata?.type === 'goal';
      const typeLabel = isGoal ? 'decomposition proposal' : 'reminder';
      const comment = `User ${reactionAction === 'approve' ? 'approved' : 'denied'} ${typeLabel}`;
      const r = await fetch(`/api/v1/reactions/${qid}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ approved: reactionAction === 'approve', comment }) });
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
    const id = w.metadata?.id || w.id;
    if (!id) return;
    setSaveRecipeLoading(true);
    try {
      const analysis = await apiClient.analyzeWantForRecipe(id);
      setSaveRecipeTarget(w);
      setSaveRecipeAnalysis(analysis);
      setShowSaveRecipeModal(true);
    } catch (e: any) {
      showNotification(`✗ Failed to analyze want: ${e.message}`);
    } finally {
      setSaveRecipeLoading(false);
    }
  };

  const handleSaveRecipeSubmit = async (metadata: RecipeMetadata, state: StateDef[]) => {
    const id = saveRecipeTarget?.metadata?.id || saveRecipeTarget?.id;
    if (!id) return;
    try {
      const r = await apiClient.saveRecipeFromWant(id, metadata, state);
      showNotification(`✓ Recipe '${r.id}' saved successfully`);
      setShowSaveRecipeModal(false);
      setSaveRecipeTarget(null);
      setSaveRecipeAnalysis(null);
    } catch (e: any) {
      showNotification(`✗ Failed: ${e.message}`);
    }
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
  const handleCloseModals = () => { sidebar.closeForm(); setEditingWant(null); setOwnerWant(null); setDeleteWantState(null); setShowDeleteConfirmation(false); setReactionWantState(null); setShowReactionConfirmation(false); setReactionAction(null); };
  
  const handleExportWants = async () => {
    setIsExporting(true);
    try {
      const response = await fetch('/api/v1/wants/export', { method: 'POST', headers: { 'Content-Type': 'application/json' } });
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
      const response = await fetch('/api/v1/wants/import', { method: 'POST', headers: { 'Content-Type': 'application/yaml' }, body: fileText });
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
    if (showBatchConfirmation) setShowBatchConfirmation(false);
    else if (expandedChain.length > 0) setExpandedChain([]);
    else if (selectedWant) { setLastSelectedWantId(selectedWant.metadata?.id || selectedWant.id || null); sidebar.clearSelection(); }
    else if (sidebar.showSummary) sidebar.closeSummary();
    else if (sidebar.showForm) sidebar.closeForm();
    else if (isSelectMode) { setSelectedWantIds(new Set()); setIsSelectMode(false); }
  };
  useEscapeKey({ onEscape: handleEscapeKey, enabled: !!selectedWant || sidebar.showSummary || sidebar.showForm || isSelectMode || expandedChain.length > 0 });

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
      } else if (e.key === 'x' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        setRadarMode(prev => !prev);
      } else if (e.key === 'g' && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        setShowGlobalState(prev => {
          if (!prev) {
            sidebar.clearSelection();
          }
          return !prev;
        });
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleCreateWant, handleToggleSelectMode, sidebar, sidebar.showForm, isSelectMode, filteredWants]);

  const getWantBackgroundImage = getBackgroundImage;

  const wantBackgroundImage = getWantBackgroundImage(selectedWant?.metadata?.type);
  const headerActions = headerState ? (
    <div className="flex items-stretch h-full">
      <div className="flex items-center px-4 border-r border-gray-100 dark:border-gray-800">
        <StatusBadge status={headerState.status} size="sm" />
      </div>
      <div className="flex items-stretch relative">
        <button
          onClick={() => sidebar.toggleHeaderAction?.('refresh')}
          className={classNames(
            "flex flex-col items-center justify-center gap-1 px-5 h-full transition-all duration-150 flex-shrink-0 focus:outline-none",
            headerState.autoRefresh
              ? "text-blue-600 dark:text-blue-400 bg-blue-50/50 dark:bg-blue-900/20"
              : "text-gray-500 hover:text-gray-700 hover:bg-gray-100 dark:text-gray-400 dark:hover:text-white dark:hover:bg-gray-800"
          )}
          title={`Reload now${headerState.autoRefresh ? ' (Auto-refresh is ON)' : ''}`}
        >
          <div className="relative">
            <RefreshCw className={classNames("h-5 w-5", (headerState.loading || headerState.autoRefresh) && "animate-spin")} />
            {headerState.autoRefresh && (
              <div className="absolute -top-1 -right-1 w-2.5 h-2.5 bg-blue-500 rounded-full border-2 border-white dark:border-gray-900 animate-pulse" />
            )}
          </div>
          <span className="text-[10px] font-bold uppercase tracking-tighter hidden sm:block">
            {headerState.autoRefresh ? 'Live' : 'Reload'}
          </span>
        </button>
        
        {/* Subtle toggle area for auto-refresh */}
        <button
          onClick={() => sidebar.toggleHeaderAction?.('autoRefresh')}
          className={classNames(
            "absolute top-0 right-0 bottom-0 w-4 hover:bg-black/5 dark:hover:bg-white/5 transition-colors flex items-center justify-center group",
            headerState.autoRefresh ? "text-blue-500" : "text-gray-300"
          )}
          title={headerState.autoRefresh ? "Disable auto-refresh" : "Enable auto-refresh"}
        >
          <div className={classNames(
            "w-1 h-4 rounded-full transition-colors",
            headerState.autoRefresh ? "bg-blue-400/50" : "bg-gray-200 dark:bg-gray-800 group-hover:bg-gray-300"
          )} />
        </button>
      </div>
    </div>
  ) : null;

  return (
    <>
      <Header
        onCreateWant={handleCreateWant}
        onCreateTargetWant={handleCreateTargetWant}
        showSummary={sidebar.showSummary}
        onSummaryToggle={sidebar.toggleSummary}
        showSelectMode={isSelectMode}
        onToggleSelectMode={handleToggleSelectMode}
        onInteractSubmit={handleInteractSubmit}
        isInteractThinking={isInteractSubmitting}
        gooseProvider={gooseProvider}
        onProviderChange={setGooseProvider}
        showMinimap={minimapOpen}
        onMinimapToggle={() => setMinimapOpen(!minimapOpen)}
        showRadarMode={radarMode}
        onRadarModeToggle={() => setRadarMode(prev => !prev)}
        showGlobalState={showGlobalState}
        onGlobalStateToggle={() => setShowGlobalState(prev => {
          if (!prev) sidebar.clearSelection();
          return !prev;
        })}
      />
      <HeaderOverlay
        isVisible={isSelectMode || showReactionConfirmation || showDeleteDraftConfirmation}
        confirmationVisible={showBatchConfirmation || showReactionConfirmation || showDeleteDraftConfirmation}
        confirmationTitle={
          showBatchConfirmation ? `Batch ${batchAction}`
          : showDeleteDraftConfirmation ? 'Delete Draft'
          : 'Confirm'
        }
        confirmationDanger={
          (showBatchConfirmation && batchAction === 'delete') ||
          showDeleteDraftConfirmation
        }
        onConfirmAction={
          showBatchConfirmation ? handleBatchConfirm
          : showDeleteDraftConfirmation ? handleDeleteDraftConfirm
          : handleReactionConfirm
        }
        onCancelAction={
          showBatchConfirmation ? handleBatchCancel
          : showDeleteDraftConfirmation ? handleDeleteDraftCancel
          : handleReactionCancel
        }
        loading={isBatchProcessing || isSubmittingReaction}
      >
        {isSelectMode && (
          <BatchActionBar
            selectedCount={selectedWantIds.size}
            onBatchStart={() => { setBatchAction('start'); setShowBatchConfirmation(true); }}
            onBatchStop={() => { setBatchAction('stop'); setShowBatchConfirmation(true); }}
            onBatchDelete={() => { setBatchAction('delete'); setShowBatchConfirmation(true); }}
            onExit={handleToggleSelectMode}
            loading={isBatchProcessing}
          />
        )}
      </HeaderOverlay>
      <main
        className="flex-1 flex overflow-hidden bg-gray-50 dark:bg-gray-950 lg:mr-[480px] mr-0 relative"
        onDragEnter={handleGlobalDragEnter}
        onDragOver={handleGlobalDragOver}
        onDragLeave={handleGlobalDragLeave}
        onDrop={handleGlobalDrop}
      >
        <div ref={cardListScrollRef} className={classNames(
          "flex-1 overflow-y-auto transition-colors duration-200",
          isGlobalDragOver && "bg-blue-50 dark:bg-blue-900/20 border-4 border-dashed border-blue-400 border-inset"
        )}>
          <div className={classNames(
            "p-3 sm:p-6 flex flex-col h-full min-h-full pb-24",
            (sidebar.showSummary || !!selectedWant || showGlobalState) ? "lg:pb-24 pb-[50vh]" : "pb-24"
          )}>
            <React.Fragment>
              {error && <div className="mb-6 p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md flex items-center"><div className="ml-3"><p className="text-sm text-red-700 dark:text-red-300">{error}</p></div><button onClick={clearError} className="ml-auto text-red-400 hover:text-red-600"><svg className="h-4 w-4" viewBox="0 0 20 20" fill="currentColor"><path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" /></svg></button></div>}
              <div className="flex-1 flex flex-col">
                <WantGrid
                  wants={regularWants} drafts={drafts} activeDraftId={activeDraftId} onDraftClick={handleDraftClick} onDraftDelete={handleDraftDelete} loading={loading} searchQuery={searchQuery} statusFilters={statusFilters} selectedWant={selectedWant} onViewWant={handleViewWant} onViewAgentsWant={handleViewAgents} onViewResultsWant={handleViewResults} onViewChatWant={handleViewChat} onEditWant={handleEditWant} onDeleteWant={handleDirectDeleteWant} onSuspendWant={handleSuspendWant} onResumeWant={handleResumeWant} onGetFilteredWants={setFilteredWants} expandedParents={expandedParents} onToggleExpand={handleToggleExpand} onCreateWant={handleCreateWant} onLabelDropped={handleLabelDropped} onWantDropped={handleWantDropped} onShowReactionConfirmation={handleShowReactionConfirmation} isSelectMode={isSelectMode} selectedWantIds={selectedWantIds} onSelectWant={handleSelectWant} correlationHighlights={correlationHighlights}
                  expandedChain={expandedChain}
                  allWants={regularWants}
                  onBubbleChildClick={handleBubbleChildClick}
                  onBubbleClose={() => setExpandedChain([])}
                />
              </div>
            </React.Fragment>
          </div>
        </div>
      </main>
      <RightSidebar
        isOpen={sidebar.showSummary || !!selectedWant || showGlobalState}
        onClose={() => {
          if (showGlobalState) { setShowGlobalState(false); return; }
          if (sidebar.showSummary) sidebar.closeSummary();
          else { sidebar.clearSelection(); setExpandedChain([]); }
        }}
        title={showGlobalState ? 'Memo' : (selectedWant ? (selectedWant.metadata?.name || selectedWant.metadata?.id || 'Want Details') : 'Summary')}
        titleIcon={showGlobalState ? StickyNote : (selectedWant ? Heart : undefined)}
        titleIconClassName={showGlobalState ? 'text-green-500' : (selectedWant ? 'text-pink-500' : undefined)}
        backgroundStyle={!showGlobalState && selectedWant ? { backgroundImage: `url(${wantBackgroundImage})`, backgroundSize: 'cover', backgroundPosition: 'center', backgroundAttachment: 'fixed' } : undefined}
        headerActions={!showGlobalState && selectedWant ? headerActions : undefined}
        disableBackdropClick={expandedChain.length > 0}
        instant
      >
        {showGlobalState ? (
          <GlobalStateSidebar />
        ) : (
          <WantDetailsSidebar
            want={selectedWant}
            initialTab={sidebarInitialTab}
            initialTabVersion={sidebarTabVersion}
            onWantUpdate={() => { if (selectedWant?.metadata?.id || selectedWant?.id) useWantStore.getState().fetchWantDetails((selectedWant.metadata?.id || selectedWant.id) as string); }}
            onHeaderStateChange={setHeaderState}
            onRegisterHeaderActions={sidebar.registerHeaderActions}
            onStart={() => startWant(selectedWant?.metadata?.id || selectedWant?.id || '')}
            onStop={() => stopWant(selectedWant?.metadata?.id || selectedWant?.id || '')}
            onSuspend={() => suspendWant(selectedWant?.metadata?.id || selectedWant?.id || '')}
            onResume={() => resumeWant(selectedWant?.metadata?.id || selectedWant?.id || '')}
            onDelete={() => { const id = selectedWant?.metadata?.id || selectedWant?.id; if (id) useWantStore.getState().setDeleteConfirmWantId(id); }}
            onSaveRecipe={() => handleSaveRecipeFromWant(selectedWant!)}
            seriesWants={seriesWants}
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
        onWantDoubleClick={handleMinimapDoubleClick}
        onDraftClick={handleMinimapDraftClick}
        isOpen={minimapOpen}
      />
      <WantForm isOpen={sidebar.showForm} onClose={handleCloseModals} editingWant={editingWant} ownerWant={ownerWant} initialTypeId={initialFormTypeId} initialItemType={initialFormItemType} mode={showRecommendationForm ? 'recommendation' : (editingWant ? 'edit' : 'create')} recommendations={recommendations} selectedRecommendation={selectedRecommendation} onRecommendationSelect={setSelectedRecommendation} onRecommendationDeploy={handleRecommendationDeploy} />

      <Toast message={notificationMessage} isVisible={isNotificationVisible} onDismiss={dismissNotification} />
      <DragOverlay />
      <SaveAsRecipeModal
        isOpen={showSaveRecipeModal}
        want={saveRecipeTarget}
        analysis={saveRecipeAnalysis}
        onClose={() => { setShowSaveRecipeModal(false); setSaveRecipeTarget(null); setSaveRecipeAnalysis(null); }}
        onSave={handleSaveRecipeSubmit}
        loading={saveRecipeLoading}
      />
      <input
        type="file"
        ref={fileInputRef}
        onChange={handleFileInputChange}
        className="hidden"
        accept=".yaml,.yml"
      />
    </>
  );
};

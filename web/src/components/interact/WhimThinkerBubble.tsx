import React, { useState, useRef } from 'react';
import { Bot, Send, ChevronDown, ChevronUp } from 'lucide-react';
import { classNames } from '@/utils/helpers';
import { Recommendation, ConversationMessage } from '@/types/interact';
import { apiClient } from '@/api/client';
import { RecommendationSelector } from './RecommendationSelector';

interface WhimThinkerBubbleProps {
  thinkerWantId: string;
  initialMessage?: string;
  onDeployed?: (wantIds: string[]) => void;
}

export const WhimThinkerBubble: React.FC<WhimThinkerBubbleProps> = ({
  thinkerWantId,
  initialMessage = '',
  onDeployed,
}) => {
  const [message, setMessage] = useState('');
  const [isComposing, setIsComposing] = useState(false);
  const [isThinking, setIsThinking] = useState(false);
  const [recommendations, setRecommendations] = useState<Recommendation[]>([]);
  const [selectedRecommendationId, setSelectedRecommendationId] = useState<string | null>(null);
  const [conversationHistory, setConversationHistory] = useState<ConversationMessage[]>([]);
  const [showHistory, setShowHistory] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isDeploying, setIsDeploying] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const handleSend = async () => {
    const msg = message.trim();
    if (!msg || isThinking) return;

    setMessage('');
    setIsThinking(true);
    setError(null);
    setRecommendations([]);
    setSelectedRecommendationId(null);

    try {
      const resp = await apiClient.sendWhimThinkerMessage(thinkerWantId, msg);
      setRecommendations(resp.recommendations || []);
      setConversationHistory(resp.conversation_history || []);
    } catch (e: any) {
      setError(e.message || 'Failed to get recommendations');
    } finally {
      setIsThinking(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey && !isComposing) {
      e.preventDefault();
      handleSend();
    }
  };

  const handleDeploy = async (recId: string) => {
    setIsDeploying(true);
    setError(null);
    try {
      const resp = await apiClient.deployWhimThinkerRecommendation(thinkerWantId, recId);
      setRecommendations([]);
      setSelectedRecommendationId(null);
      onDeployed?.(resp.want_ids);
    } catch (e: any) {
      setError(e.message || 'Failed to deploy');
    } finally {
      setIsDeploying(false);
    }
  };

  const placeholder = initialMessage
    ? `「${initialMessage.substring(0, 20)}${initialMessage.length > 20 ? '…' : ''}」について聞く`
    : 'アイディアを聞いてみる…';

  return (
    <div className="mt-2 border-t border-gray-200 dark:border-gray-700 pt-2 space-y-2">
      {/* Chat input */}
      <div className="flex items-center gap-2">
        <div className="flex items-center justify-center h-8 w-8 rounded-full bg-blue-600 flex-shrink-0">
          <Bot className="h-4 w-4 text-white" />
        </div>
        <div className={classNames(
          'relative flex items-center gap-2 px-3 py-1.5 bg-white dark:bg-gray-800 rounded-xl shadow border-2 flex-1 min-w-0',
          isThinking ? 'border-blue-300 dark:border-blue-700' : 'border-blue-500 dark:border-blue-600'
        )}>
          {isThinking ? (
            <span className="text-xs text-gray-500 dark:text-gray-400 animate-pulse">考え中…</span>
          ) : (
            <>
              <input
                ref={inputRef}
                type="text"
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                onKeyDown={handleKeyDown}
                onCompositionStart={() => setIsComposing(true)}
                onCompositionEnd={() => setIsComposing(false)}
                placeholder={placeholder}
                className="bg-transparent border-none outline-none text-xs text-gray-900 dark:text-gray-100 placeholder-gray-400 flex-1 min-w-0 focus:ring-0"
                onClick={(e) => e.stopPropagation()}
              />
              {message && (
                <button
                  onClick={(e) => { e.stopPropagation(); handleSend(); }}
                  className="text-blue-600 hover:text-blue-700 flex-shrink-0"
                >
                  <Send className="h-3.5 w-3.5" />
                </button>
              )}
            </>
          )}
        </div>
      </div>

      {/* Error */}
      {error && (
        <p className="text-xs text-red-500 pl-10">{error}</p>
      )}

      {/* Recommendations */}
      {recommendations.length > 0 && (
        <div className="pl-10 space-y-2" onClick={(e) => e.stopPropagation()}>
          <RecommendationSelector
            recommendations={recommendations}
            selectedId={selectedRecommendationId}
            onSelect={(rec) => setSelectedRecommendationId(rec.id)}
          />
          {selectedRecommendationId && (
            <button
              onClick={() => handleDeploy(selectedRecommendationId)}
              disabled={isDeploying}
              className="w-full py-1.5 px-3 bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium rounded-lg disabled:opacity-50 transition-colors"
            >
              {isDeploying ? '追加中…' : '選択したWantを追加'}
            </button>
          )}
        </div>
      )}

      {/* Conversation history toggle */}
      {conversationHistory.length > 0 && (
        <div className="pl-10">
          <button
            onClick={(e) => { e.stopPropagation(); setShowHistory(h => !h); }}
            className="flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300"
          >
            {showHistory ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
            会話履歴 ({conversationHistory.length})
          </button>
          {showHistory && (
            <div className="mt-1 space-y-1 max-h-40 overflow-y-auto pr-1">
              {conversationHistory.map((msg, i) => (
                <div
                  key={i}
                  className={classNames(
                    'text-xs px-2 py-1 rounded',
                    msg.role === 'user'
                      ? 'bg-blue-50 dark:bg-blue-900/30 text-blue-900 dark:text-blue-100 ml-4'
                      : 'bg-gray-50 dark:bg-gray-800 text-gray-700 dark:text-gray-300 mr-4'
                  )}
                >
                  <span className="font-medium text-[10px] uppercase tracking-wide opacity-60 mr-1">
                    {msg.role === 'user' ? 'あなた' : 'AI'}
                  </span>
                  {msg.content}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

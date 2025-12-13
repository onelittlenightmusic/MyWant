import React, { useState } from 'react';
import { Eye, Play } from 'lucide-react';
import { WantTypeDefinition } from '@/types/wantType';
import { classNames } from '@/utils/helpers';
import { apiClient } from '@/api/client';

interface WantTypeControlPanelProps {
  selectedWantType: WantTypeDefinition | null;
  onViewDetails?: (wantType: WantTypeDefinition) => void;
  onDeploySuccess?: (message: string) => void;
  onDeployError?: (error: string) => void;
  loading?: boolean;
  sidebarMinimized?: boolean;
}

export const WantTypeControlPanel: React.FC<WantTypeControlPanelProps> = ({
  selectedWantType,
  onViewDetails,
  onDeploySuccess,
  onDeployError,
  loading = false,
  sidebarMinimized = false
}) => {
  const [deploying, setDeploying] = useState(false);

  // Check if want type has examples for deployment
  const hasExamples = selectedWantType?.examples && selectedWantType.examples.length > 0;

  // Button enable/disable logic
  const canDeploy = selectedWantType && hasExamples && !loading && !deploying;
  const canViewDetails = selectedWantType && onViewDetails;

  const handleDeploy = async () => {
    if (!selectedWantType || !hasExamples) return;

    setDeploying(true);
    try {
      // Use the first example to create a want
      const example = selectedWantType.examples[0];

      // Create a want from the example
      const want = {
        metadata: {
          name: `${selectedWantType.metadata.name}-${Date.now()}`,
          type: selectedWantType.metadata.name,
        },
        spec: {
          params: example.want?.spec?.params || {},
        },
      };

      // Deploy the want
      await apiClient.createWant(want);

      onDeploySuccess?.(
        `Want type "${selectedWantType.metadata.name}" deployed successfully!`
      );
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : 'Failed to deploy want type';
      onDeployError?.(errorMessage);
      console.error('Deploy error:', error);
    } finally {
      setDeploying(false);
    }
  };

  return (
    <div className={classNames(
      "fixed bottom-0 right-0 bg-blue-50 border-t border-blue-200 shadow-lg z-30 transition-all duration-300 ease-in-out",
      sidebarMinimized ? "lg:left-20" : "lg:left-64",
      "left-0"
    )}>
      <div className="px-6 py-3">
        <div className="flex items-center justify-start gap-4">
          {/* Control Buttons */}
          <div className="flex items-center space-x-2 flex-wrap gap-y-2">
            {/* Deploy */}
            <button
              onClick={handleDeploy}
              disabled={!canDeploy}
              className={classNames(
                'flex items-center space-x-2 px-4 py-2 rounded-md text-sm font-medium transition-colors',
                canDeploy
                  ? 'bg-green-600 text-white hover:bg-green-700'
                  : 'bg-gray-100 text-gray-400 cursor-not-allowed'
              )}
              title={
                !selectedWantType
                  ? 'No want type selected'
                  : !hasExamples
                  ? 'Want type has no examples for deployment'
                  : 'Deploy want type with example configuration'
              }
            >
              <Play className="h-4 w-4" />
              <span>{deploying ? 'Deploying...' : 'Deploy'}</span>
            </button>

            {/* View Details */}
            <button
              onClick={() => canViewDetails && onViewDetails(selectedWantType!)}
              disabled={!canViewDetails}
              className={classNames(
                'flex items-center space-x-2 px-4 py-2 rounded-md text-sm font-medium transition-colors',
                canViewDetails
                  ? 'bg-blue-600 text-white hover:bg-blue-700'
                  : 'bg-gray-100 text-gray-400 cursor-not-allowed'
              )}
              title={canViewDetails ? 'View want type details' : 'No want type selected'}
            >
              <Eye className="h-4 w-4" />
              <span>View Details</span>
            </button>
          </div>

          {/* Selected Info */}
          <div className="flex-1 min-w-0">
            {selectedWantType ? (
              <p className="text-sm font-medium text-gray-700">
                {selectedWantType.metadata.name}
                {hasExamples && (
                  <span className="ml-2 inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                    Ready to Deploy
                  </span>
                )}
              </p>
            ) : (
              <p className="text-sm text-gray-500">No want type selected</p>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

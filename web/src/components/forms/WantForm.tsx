import React, { useState, useEffect } from 'react';
import { X, Save, FileText, Code } from 'lucide-react';
import { Want } from '@/types/want';
import { YamlEditor } from './YamlEditor';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { validateYaml, formatYaml, stringifyYaml } from '@/utils/yaml';
import { useWantStore } from '@/stores/wantStore';

interface WantFormProps {
  isOpen: boolean;
  onClose: () => void;
  editingWant?: Want | null;
}

const SAMPLE_YAML = `wants:
  # Packet generator
  - metadata:
      name: "number generator"
      type: "numbers"
      labels:
        role: "source"
        stream: "primary"
    spec:
      params:
        rate: 2.0
        count: 100

  # Processing queue
  - metadata:
      name: "processing queue"
      type: "queue"
      labels:
        role: "processor"
    spec:
      params:
        service_time: 0.1
      using:
        - role: "source"

  # Final collector
  - metadata:
      name: "result collector"
      type: "sink"
      labels:
        role: "terminal"
    spec:
      params: {}
      using:
        - role: "processor"
`;

const FIBONACCI_EXAMPLE = `wants:
  # Number generator for fibonacci sequence
  - metadata:
      name: "fibonacci generator"
      type: "numbers"
      labels:
        role: "source"
        target: "fibonacci sequence"
    spec:
      params:
        count: 20
        rate: 1.0

  # Fibonacci sequence processor
  - metadata:
      name: "fibonacci processor"
      type: "fibonacci sequence"
      labels:
        role: "processor"
        category: "math"
    spec:
      params:
        max_iterations: 20
      using:
        - role: "source"

  # Result collector
  - metadata:
      name: "fibonacci collector"
      type: "collector"
      labels:
        role: "terminal"
    spec:
      params: {}
      using:
        - role: "processor"
`;

const TRAVEL_EXAMPLE = `wants:
  # Travel request generator
  - metadata:
      name: "travel request generator"
      type: "numbers"
      labels:
        role: "source"
        category: "travel"
        target: "travel itinerary planner"
    spec:
      params:
        count: 5
        rate: 1.0

  # Travel itinerary planner
  - metadata:
      name: "travel planner"
      type: "travel itinerary planner"
      labels:
        role: "processor"
        category: "planning"
    spec:
      params:
        destination: "Tokyo"
        duration_days: 3
        budget: 2000
      using:
        - role: "source"

  # Travel result collector
  - metadata:
      name: "travel collector"
      type: "collector"
      labels:
        role: "terminal"
        category: "results"
    spec:
      params: {}
      using:
        - role: "processor"
`;

export const WantForm: React.FC<WantFormProps> = ({
  isOpen,
  onClose,
  editingWant
}) => {
  const { createWantFromYaml, updateWant, loading, error } = useWantStore();
  const [yaml, setYaml] = useState(SAMPLE_YAML);
  const [name, setName] = useState('');
  const [validationError, setValidationError] = useState<string | null>(null);
  const [isEditing, setIsEditing] = useState(false);

  // Initialize form when editing
  useEffect(() => {
    if (editingWant) {
      setIsEditing(true);
      // Convert want structure to YAML config format for editing
      const configForEdit = {
        wants: [{
          metadata: {
            name: editingWant.metadata?.name,
            type: editingWant.metadata?.type,
            labels: editingWant.metadata?.labels || {}
          },
          spec: {
            params: editingWant.spec?.params || {},
            ...(editingWant.spec?.using && { using: editingWant.spec.using }),
            ...(editingWant.spec?.recipe && { recipe: editingWant.spec.recipe })
          }
        }]
      };
      setYaml(stringifyYaml(configForEdit));
      setName(editingWant.metadata?.name || editingWant.metadata?.id || 'Unnamed Want');
    } else {
      setIsEditing(false);
      setYaml(SAMPLE_YAML);
      setName('');
    }
    setValidationError(null);
  }, [editingWant, isOpen]);

  // Validate YAML on change
  useEffect(() => {
    if (yaml.trim()) {
      const validation = validateYaml(yaml);
      setValidationError(validation.valid ? null : validation.error || 'Invalid YAML');
    } else {
      setValidationError(null);
    }
  }, [yaml]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!yaml.trim()) {
      setValidationError('YAML configuration is required');
      return;
    }

    const validation = validateYaml(yaml);
    if (!validation.valid) {
      setValidationError(validation.error || 'Invalid YAML');
      return;
    }

    try {
      if (isEditing && editingWant) {
        await updateWant(editingWant.id, yaml);
      } else {
        await createWantFromYaml(yaml, name || undefined);
      }
      onClose();
    } catch (error) {
      console.error('Failed to save want:', error);
    }
  };

  const handleFormatYaml = () => {
    try {
      const formatted = formatYaml(yaml);
      setYaml(formatted);
    } catch (error) {
      console.error('Failed to format YAML:', error);
    }
  };

  const loadSample = (sampleType: 'qnet' | 'fibonacci' | 'travel' = 'qnet') => {
    const samples = {
      qnet: SAMPLE_YAML,
      fibonacci: FIBONACCI_EXAMPLE,
      travel: TRAVEL_EXAMPLE
    };
    setYaml(samples[sampleType]);
    setName('');
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
      <div className="relative top-20 mx-auto p-5 border w-11/12 md:w-3/4 lg:w-2/3 xl:w-1/2 shadow-lg rounded-md bg-white">
        {/* Header */}
        <div className="flex items-center justify-between pb-4 border-b border-gray-200">
          <h3 className="text-lg font-semibold text-gray-900">
            {isEditing ? 'Edit Want' : 'Create New Want'}
          </h3>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            <X className="h-6 w-6" />
          </button>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} className="mt-6">
          {/* Name field (only for new wants) */}
          {!isEditing && (
            <div className="mb-6">
              <label className="label">
                Want Name (optional)
              </label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Enter a descriptive name..."
                className="input"
              />
              <p className="mt-1 text-sm text-gray-500">
                If not provided, a name will be generated from the configuration.
              </p>
            </div>
          )}

          {/* YAML Editor */}
          <div className="mb-6">
            <div className="flex items-center justify-between mb-2">
              <label className="label">
                YAML Configuration
              </label>
              <div className="flex space-x-2">
                {!isEditing && (
                  <div className="relative group">
                    <button
                      type="button"
                      onClick={() => loadSample()}
                      className="inline-flex items-center px-3 py-1 border border-gray-300 shadow-sm text-sm leading-4 font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50"
                    >
                      <FileText className="h-4 w-4 mr-1" />
                      Samples
                    </button>

                    <div className="absolute left-0 top-8 w-48 bg-white rounded-md shadow-lg border border-gray-200 z-10 opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all duration-200">
                      <div className="py-1">
                        <button
                          type="button"
                          onClick={() => loadSample('qnet')}
                          className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                        >
                          Queue Network (QNet)
                        </button>
                        <button
                          type="button"
                          onClick={() => loadSample('fibonacci')}
                          className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                        >
                          Fibonacci Sequence
                        </button>
                        <button
                          type="button"
                          onClick={() => loadSample('travel')}
                          className="flex items-center w-full px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                        >
                          Travel Planning
                        </button>
                      </div>
                    </div>
                  </div>
                )}
                <button
                  type="button"
                  onClick={handleFormatYaml}
                  className="inline-flex items-center px-3 py-1 border border-gray-300 shadow-sm text-sm leading-4 font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50"
                >
                  <Code className="h-4 w-4 mr-1" />
                  Format
                </button>
              </div>
            </div>

            <YamlEditor
              value={yaml}
              onChange={setYaml}
              height="500px"
              className="border border-gray-300 rounded-md"
            />

            {validationError && (
              <p className="mt-2 text-sm text-red-600">
                {validationError}
              </p>
            )}
          </div>

          {/* Error message */}
          {error && (
            <div className="mb-6 p-3 bg-red-50 border border-red-200 rounded-md">
              <p className="text-sm text-red-600">{error}</p>
            </div>
          )}

          {/* Actions */}
          <div className="flex items-center justify-end space-x-3 pt-4 border-t border-gray-200">
            <button
              type="button"
              onClick={onClose}
              className="btn-secondary"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={loading || !!validationError || !yaml.trim()}
              className="btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {loading ? (
                <>
                  <LoadingSpinner size="sm" color="white" className="mr-2" />
                  {isEditing ? 'Updating...' : 'Creating...'}
                </>
              ) : (
                <>
                  <Save className="h-4 w-4 mr-2" />
                  {isEditing ? 'Update Want' : 'Create Want'}
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
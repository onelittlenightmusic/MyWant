import React from 'react';
import { AlertTriangle, XCircle, AlertCircle, Copy } from 'lucide-react';
import { ApiError } from '@/types/api';

interface ErrorDisplayProps {
  error: ApiError | string;
  className?: string;
  showCopy?: boolean;
}

export const ErrorDisplay: React.FC<ErrorDisplayProps> = ({
  error,
  className = '',
  showCopy = true
}) => {
  const errorObj = typeof error === 'string' ? { message: error, status: 500 } : error;

  const getErrorIcon = () => {
    if (errorObj.type === 'validation') return AlertTriangle;
    if (errorObj.status >= 500) return XCircle;
    return AlertCircle;
  };

  const getErrorColor = () => {
    if (errorObj.type === 'validation') return 'border-yellow-200 bg-yellow-50 text-yellow-800';
    if (errorObj.status >= 500) return 'border-red-200 bg-red-50 text-red-800';
    return 'border-orange-200 bg-orange-50 text-orange-800';
  };

  const handleCopyError = () => {
    const errorText = errorObj.details || errorObj.message;
    navigator.clipboard.writeText(errorText);
  };

  const Icon = getErrorIcon();

  return (
    <div className={`p-4 border rounded-md ${getErrorColor()} ${className}`}>
      <div className="flex items-start">
        <Icon className="h-5 w-5 mt-0.5 mr-3 flex-shrink-0" />
        <div className="flex-1 min-w-0">
          <h3 className="text-sm font-medium">
            {errorObj.type === 'validation' ? 'Configuration Error' : 'Request Failed'}
          </h3>
          <div className="mt-2 text-sm">
            {errorObj.type === 'validation' && errorObj.details ? (
              <div className="space-y-2">
                <p>{errorObj.details}</p>
                {errorObj.details.includes('Available standard types:') && (
                  <div className="bg-white bg-opacity-50 p-3 rounded border">
                    <p className="font-medium mb-2">This error occurs when:</p>
                    <ul className="list-disc list-inside space-y-1 text-xs">
                      <li>A want type is misspelled or doesn't exist</li>
                      <li>A custom type hasn't been registered</li>
                      <li>The type name doesn't match any available types</li>
                    </ul>
                  </div>
                )}
              </div>
            ) : (
              <p>{errorObj.message}</p>
            )}
          </div>
          {showCopy && (errorObj.details || errorObj.message) && (
            <button
              onClick={handleCopyError}
              className="mt-2 inline-flex items-center text-xs hover:underline"
            >
              <Copy className="h-3 w-3 mr-1" />
              Copy error details
            </button>
          )}
        </div>
      </div>
    </div>
  );
};
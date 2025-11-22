import * as yaml from 'js-yaml';
import { WantConfig } from '@/types/want';

export interface WantTypeParameter {
  name: string;
  type: string;
  required: boolean;
  description: string;
  default?: any;
}

export interface WantTypeDefinition {
  metadata: {
    name: string;
    title: string;
    description: string;
    version: string;
    category: string;
    pattern: string;
  };
  parameters: WantTypeParameter[];
  state: any[];
  connectivity: any;
}

export const parseYaml = (yamlString: string): WantConfig => {
  try {
    return yaml.load(yamlString) as WantConfig;
  } catch (error) {
    throw new Error(`Invalid YAML: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
};

export const stringifyYaml = (data: any): string => {
  try {
    return yaml.dump(data, {
      indent: 2,
      lineWidth: 120,
      noRefs: true,
    });
  } catch (error) {
    throw new Error(`Failed to serialize YAML: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
};

/**
 * Validate YAML structure against backend want type specification
 */
export const validateYamlWithSpec = (
  yamlString: string,
  _wantType: string,
  spec?: WantTypeDefinition
): { isValid: boolean; error?: string; data?: any } => {
  // First, parse and validate basic YAML syntax
  let data;
  try {
    data = yaml.load(yamlString);
  } catch (error) {
    return {
      isValid: false,
      error: error instanceof Error ? error.message : 'Unknown error',
    };
  }

  if (!data || typeof data !== 'object') {
    return { isValid: false, error: 'Want configuration must be an object' };
  }

  const metadata = (data as any).metadata;
  const specObj = (data as any).spec;

  // Validate metadata structure
  if (!metadata || typeof metadata !== 'object') {
    return { isValid: false, error: 'metadata field is required and must be an object' };
  }

  // Validate labels if present
  const labels = metadata.labels;
  if (labels !== undefined && labels !== null) {
    if (typeof labels !== 'object' || Array.isArray(labels)) {
      return {
        isValid: false,
        error: 'metadata.labels must be a map/object with key-value pairs (e.g., "role: producer"), not a string, number, or array.',
      };
    }
  }

  // Validate against want type spec if provided
  if (spec && spec.parameters && Array.isArray(spec.parameters)) {
    const params = (specObj?.params || {}) as Record<string, any>;

    // Check for invalid parameters
    const validParamNames = new Set(spec.parameters.map(p => p.name));
    for (const paramName of Object.keys(params)) {
      if (!validParamNames.has(paramName)) {
        return {
          isValid: false,
          error: `spec.params contains unknown parameter "${paramName}". Valid parameters are: ${Array.from(validParamNames).join(', ')}`,
        };
      }
    }

    // Check for required parameters
    const missingRequired: string[] = [];
    for (const param of spec.parameters) {
      if (param.required && params[param.name] === undefined) {
        missingRequired.push(param.name);
      }
    }
    if (missingRequired.length > 0) {
      return {
        isValid: false,
        error: `Missing required parameters: ${missingRequired.join(', ')}`,
      };
    }
  }

  return { isValid: true, data };
};

export const validateYaml = (yamlString: string): { isValid: boolean; error?: string; data?: any } => {
  try {
    const data = yaml.load(yamlString);

    // Validate semantic structure
    if (data && typeof data === 'object') {
      const metadata = (data as any).metadata;
      if (metadata && typeof metadata === 'object') {
        const labels = metadata.labels;
        // Check if labels exists and is not a proper map/object
        if (labels !== undefined && labels !== null) {
          if (typeof labels !== 'object' || Array.isArray(labels)) {
            return {
              isValid: false,
              error: 'metadata.labels must be a map/object with key-value pairs (e.g., "role: producer"), not a string, number, or array.',
            };
          }
        }
      }
    }

    return { isValid: true, data };
  } catch (error) {
    return {
      isValid: false,
      error: error instanceof Error ? error.message : 'Unknown error',
    };
  }
};

export const formatYaml = (yamlString: string): string => {
  try {
    const parsed = yaml.load(yamlString);
    return yaml.dump(parsed, {
      indent: 2,
      lineWidth: 120,
      noRefs: true,
    });
  } catch (error) {
    // Return original if formatting fails
    return yamlString;
  }
};
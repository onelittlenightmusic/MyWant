import * as yaml from 'js-yaml';
import { WantConfig } from '@/types/want';

export const parseYaml = (yamlString: string): WantConfig => {
  try {
    return yaml.load(yamlString) as WantConfig;
  } catch (error) {
    throw new Error(`Invalid YAML: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
};

export const stringifyYaml = (config: WantConfig): string => {
  try {
    return yaml.dump(config, {
      indent: 2,
      lineWidth: 120,
      noRefs: true,
    });
  } catch (error) {
    throw new Error(`Failed to serialize YAML: ${error instanceof Error ? error.message : 'Unknown error'}`);
  }
};

export const validateYaml = (yamlString: string): { valid: boolean; error?: string } => {
  try {
    yaml.load(yamlString);
    return { valid: true };
  } catch (error) {
    return {
      valid: false,
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
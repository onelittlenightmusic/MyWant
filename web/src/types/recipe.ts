export interface RecipeMetadata {
  name: string;
  description?: string;
  version?: string;
  type?: string;
  custom_type?: string;
}

export interface RecipeWant {
  metadata?: {
    name?: string;
    type?: string;
    labels?: Record<string, string>;
  };
  spec?: {
    params?: Record<string, any>;
  };
  // Legacy flattened fields
  name?: string;
  type?: string;
  labels?: Record<string, string>;
  params?: Record<string, any>;
  using?: Record<string, string>[];
  requires?: string[];
  recipeAgent?: boolean;
}

export interface RecipeResultSpec {
  want_name: string;
  stat_name: string;
  description?: string;
}

export interface RecipeExample {
  wants: RecipeWant[];
}

export interface RecipeContent {
  metadata: RecipeMetadata;
  parameters?: Record<string, any>;
  wants: RecipeWant[];
  result?: RecipeResultSpec[];
  example?: RecipeExample;
}

export interface GenericRecipe {
  recipe: RecipeContent;
}

export interface RecipeCreateResponse {
  id: string;
  message: string;
}

export interface RecipeUpdateResponse {
  id: string;
  message: string;
}

export interface RecipeListResponse {
  [key: string]: GenericRecipe;
}
// @ts-ignore
/* eslint-disable */
import { request } from '@umijs/max';
import { LlmPrompt, QueryLlmScenesParams } from './types';

const QUERY_LLM_SCENES = `
  query QueryLlmScenes($input: QueryLlmScenesInput!) {
    Result: LlmScenes(input: $input) {
      totalCount
      list {
        id
        key
        name
        description
        config { temperature topP maxTokens maxCompletionTokens }
        messages { role content name }
        timeout
        responseFormat { type jsonSchema { name strict schema } }
        enabled
        createdAt
        updatedAt
      }
    }
  }
`;

const GET_LLM_SCENE = `
  query GetLlmScene($input: GetLlmSceneInput!) {
    Result: LlmScene(input: $input) {
      scene {
        id
        key
        name
        description
        config { temperature topP maxTokens maxCompletionTokens }
        messages { role content name }
        timeout
        responseFormat { type jsonSchema { name strict schema } }
        enabled
        createdAt
        updatedAt
      }
      prompts {
        id
        sceneKey
        platform
        name
        model
        providers
        config { temperature topP maxTokens maxCompletionTokens }
        messages { role content name }
        timeout
        weight
        variants
        enabled
        createdAt
        updatedAt
      }
    }
  }
`;

const CREATE_LLM_SCENE = `
  mutation CreateLlmScene($input: CreateLlmSceneInput!) {
    Result: CreateLlmScene(input: $input) {
      id
      key
      name
      description
      config { temperature topP maxTokens maxCompletionTokens }
      messages { role content name }
      timeout
      responseFormat { type jsonSchema { name strict schema } }
      enabled
      createdAt
      updatedAt
    }
  }
`;

const UPDATE_LLM_SCENE = `
  mutation UpdateLlmScene($input: UpdateLlmSceneInput!) {
    Result: UpdateLlmScene(input: $input) {
      id
      key
      name
      description
      config { temperature topP maxTokens maxCompletionTokens }
      messages { role content name }
      timeout
      responseFormat { type jsonSchema { name strict schema } }
      enabled
      createdAt
      updatedAt
    }
  }
`;

const DELETE_LLM_SCENE = `
  mutation DeleteLlmScene($input: DeleteLlmSceneInput!) {
    Result: DeleteLlmScene(input: $input)
  }
`;

const CREATE_LLM_PROMPT = `
  mutation CreateLlmPrompt($input: CreateLlmPromptInput!) {
    Result: CreateLlmPrompt(input: $input) {
      id
      sceneKey
      platform
      name
      model
      providers
      config { temperature topP maxTokens maxCompletionTokens }
      messages { role content name }
      timeout
      weight
      variants
      enabled
      createdAt
      updatedAt
    }
  }
`;

const UPDATE_LLM_PROMPT = `
  mutation UpdateLlmPrompt($input: UpdateLlmPromptInput!) {
    Result: UpdateLlmPrompt(input: $input) {
      id
      sceneKey
      platform
      name
      model
      providers
      config { temperature topP maxTokens maxCompletionTokens }
      messages { role content name }
      timeout
      weight
      variants
      enabled
      createdAt
      updatedAt
    }
  }
`;

const DELETE_LLM_PROMPT = `
  mutation DeleteLlmPrompt($input: DeleteLlmPromptInput!) {
    Result: DeleteLlmPrompt(input: $input)
  }
`;

const QUERY_LLM_PROMPTS = `
  query QueryLlmPrompts($input: QueryLlmPromptsInput!) {
    Result: LlmPrompts(input: $input) {
      totalCount
      list {
        id
        sceneId
        sceneKey
        platform
        name
        model
        providers
        config { temperature topP maxTokens maxCompletionTokens }
        messages { role content name }
        timeout
        weight
        variants
        enabled
        createdAt
        updatedAt
      }
    }
  }
`;

export async function queryLlmScenes(params: QueryLlmScenesParams) {
  const pageSize = params.pageSize || 10;
  const input: any = {
    limit: pageSize,
    offset: ((params?.current || 1) - 1) * pageSize,
    enabled: params.enabled,
  };
  Object.keys(input).forEach((key) => {
    if (input[key] === undefined || input[key] === null || input[key] === '') {
      delete input[key];
    }
  });
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_LLM_SCENES,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result;
}

export async function getLlmScene(sceneId: string, withPrompts = true) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: GET_LLM_SCENE,
      variables: {
        input: { id: sceneId, withPrompts },
      },
    }),
  });
  return response.data?.Result;
}

export async function createLlmScene(params: any) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: CREATE_LLM_SCENE,
      variables: {
        input: params,
      },
    }),
  });
}

export async function updateLlmScene(params: any) {
  delete params.createdAt;
  delete params.updatedAt;
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: UPDATE_LLM_SCENE,
      variables: {
        input: params,
      },
    }),
  });
}

export async function deleteLlmScene(sceneId: string) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: DELETE_LLM_SCENE,
      variables: {
        input: { id: sceneId },
      },
    }),
  });
}

export async function createLlmPrompt(params: any) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: CREATE_LLM_PROMPT,
      variables: {
        input: params,
      },
    }),
  });
}

export async function updateLlmPrompt(params: any) {
  if (!params.id) {
    return Promise.reject(new Error('id is required'));
  }
  let variants = null;
  if (params?.variants != undefined) {
    variants = {
      values: params?.variants,
    };
  }
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: UPDATE_LLM_PROMPT,
      variables: {
        input: {
          id: params.id,
          name: params?.name,
          timeout: params?.timeout,
          enabled: params?.enabled,
          weight: params?.weight,
          variants: variants,
        },
      },
    }),
  });
}

export async function deleteLlmPrompt(promptId: string) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: DELETE_LLM_PROMPT,
      variables: {
        input: { id: promptId },
      },
    }),
  });
}

export async function queryLlmPrompts(params: {
  sceneId?: string;
  enabled?: boolean;
  current?: number;
  pageSize?: number;
}) {
  const pageSize = params.pageSize || 100;
  const input: any = {
    limit: pageSize,
    offset: ((params?.current || 1) - 1) * pageSize,
    enabled: params.enabled,
    sceneId: params.sceneId,
  };
  Object.keys(input).forEach((key) => {
    if (input[key] === undefined || input[key] === null || input[key] === '') {
      delete input[key];
    }
  });
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_LLM_PROMPTS,
      variables: {
        input,
      },
    }),
  });
  return response.data?.Result;
}

const TEST_PROMPT = `
  mutation SceneTest($input: SceneTestInput!) {
    Result: SceneTest(input: $input) {
      success
      result
      error
      duration
      completionId
      usage {
        promptTokens
        completionTokens
        totalTokens
      }
      metadata {
        sceneKey
        sceneId
        promptId
        model
        provider
      }
    }
  }
`;

export async function testPrompt(params: {
  sceneId: string;
  byVariant?: string;
  byPromptId?: string;
  byPrompt?: LlmPrompt;
  variables: string;
}) {
  return await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: TEST_PROMPT,
      variables: {
        input: {
          sceneId: params.sceneId,
          byVariant: params.byVariant,
          byPromptId: params.byPromptId,
          byPrompt: params?.byPrompt
            ? {
                platform: params.byPrompt.platform,
                name: params.byPrompt.name,
                model: params.byPrompt.model,
                providers: params.byPrompt.providers,
                config: params.byPrompt.config,
                messages: params.byPrompt.messages,
                timeout: params.byPrompt.timeout,
              }
            : undefined,
          variables: params.variables,
        },
      },
    }),
  });
}

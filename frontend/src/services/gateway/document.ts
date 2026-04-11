// 文档相关类型定义
import { request } from '@umijs/max';
import dayjs from 'dayjs';
import { z } from 'zod';

export enum DocumentCatalog {
  UNSPECIFIED = 'unspecified',
  AIRDROP = 'airdrop',
  API = 'api',
  CRYPTOCURRENCY_LISTING = 'cryptocurrencyListing',
  CRYPTOCURRENCY_DELISTING = 'cryptocurrencyDelisting',
  ACTIVITY = 'activity',
  NEWS = 'news',
  FLASH_NEWS = 'flashNews',
  OTHER = 'other',
}

export const DocumentCatalogOptions = [
  { label: 'Airdrop', value: DocumentCatalog.AIRDROP },
  { label: 'API', value: DocumentCatalog.API },
  { label: 'Cryptocurrency Listing', value: DocumentCatalog.CRYPTOCURRENCY_LISTING },
  { label: 'Cryptocurrency Delisting', value: DocumentCatalog.CRYPTOCURRENCY_DELISTING },
  { label: 'Activity', value: DocumentCatalog.ACTIVITY },
  { label: 'News', value: DocumentCatalog.NEWS },
  { label: 'Flash News', value: DocumentCatalog.FLASH_NEWS },
  { label: 'Other', value: DocumentCatalog.OTHER },
];

export enum DocumentFormat {
  UNSPECIFIED = 'unspecified',
  MARKDOWN = 'markdown',
  TXT = 'txt',
  HTML = 'html',
}

export enum DocumentStatus {
  UNSPECIFIED = 'unspecified',
  DRAFT = 'draft',
  DRAFT_FAILED = 'draftFailed',
  PENDING = 'pending',
  PENDING_FAILED = 'pendingFailed',
  ACTIVE = 'active',
  ARCHIVED = 'archived',
  DEDUPED = 'deduped',
  TIMEOUT = 'timeout',
}

export interface Document {
  id: string;
  source: string;
  provider: string;
  catalog: DocumentCatalog;
  title: string;
  content: string;
  aiTitle: string;
  aiSummary: string;
  aiTags: string[];
  aiCoins: string[];
  aiInfluence: string;
  aiInfluenceScore: number;
  aiSentiment: number;
  lang: string;
  md5: string;
  url: string;
  authors: string[];
  format: DocumentFormat;
  status: DocumentStatus;
  errMsg: string;
  dedupedBy: string;
  publishedAt: number;
  createdAt: number;
  updatedAt: number;
}

export interface DocumentsConnection {
  totalCount: number;
  list: Document[];
}

export interface DocumentSimilarityResult {
  leftId: string;
  rightId: string;
  similarity: number;
}

export type QueryDocumentParams = API.PageParams & {
  id?: string;
  keyword?: string;
  source?: string;
  catalog?: DocumentCatalog;
  provider?: string;
  publishedAtRange?: string[];
  status?: DocumentStatus;
  tag?: string;
  coin?: string;
  influenceScore?: number;
  sentiment?: number;
};

const SourceMap: Record<string, string> = {
  binance: '币安',
  okx: '欧易',
  jin10: '金十数据',
  jinse: '金色财经',
  theblockbeats: '律动',
  tmtpost: '钛媒体',
  slowmist: '慢雾',
  bloomberg: '彭博社',
  gelonghui: '格隆汇',
  foresightnews: 'Foresight News',
  wallstreet: '华尔街见闻',
  cailianshe: '财联社',
  followin: 'Followin',
  fastbull: 'FastBull',
  twitter: 'Twitter',
  huxiu: '虎嗅',
  coindesk: 'CoinDesk',
  panews: 'PANews',
  zaobao: '联合早报',
  wublock: '吴说区块链',
  cointime: 'CoinTime',
  cjmb: '财经慢报',
  chaincatcher: '链捕手',
  cjkx: '财经快讯',
  techflow: 'TechFlow',
  bqkx: '币圈快讯',
  bimi: '币㊙️快讯',
  xhqcankao: '风向旗参考快讯',
  ppbbb: '币圈新闻即时快讯🅥',
  loopdns: 'LoopDNS资讯播报',
  cjzx: '财经资讯',
  odaily: 'Odaily',
  lslbd: '链上老币登',
  bitpush: 'Bitpush',
  zhuxinshe: '竹新社',
  zaihuapd: '科技圈🎗在花频道📮',
  BWEnews: '方程式新闻',
  GodlyNews: 'Yummy 😋',
  fencha: '分叉财经',
};


export enum CalendarSource {
  UNSPECIFIED = 'unspecified',
  GATEIO = 'gateio',
  JIN10 = 'jin10',
}

export enum CalendarType {
  UNSPECIFIED = 'unspecified',
  ECONOMIC_DATA = 'economic_data',
  PROJECT_EVENT = 'project_event',
  TOKEN_UNLOCK = 'token_unlock',
  SUMMIT_EVENT = 'summit_event',
  FINANCING = 'financing',
  EVENTS = 'events',
  OTHER = 'other',
}

export interface EconomicCalendarExtention {
  unit: string;
  actual: string;
  previous: string;
  consensus: string;
}

export type CalendarExtention = EconomicCalendarExtention | null;

export interface CalendarItem {
  id: string;
  dateId: number;
  source: CalendarSource;
  sid: string;
  type: CalendarType;
  category: string;
  country?: string;
  project?: string;
  symbol?: string;
  title: string;
  content: string;
  importance: number;
  url?: string;
  ext?: CalendarExtention;
  publishedAt: number;
  createdAt: number;
  updatedAt: number;
}

export interface QueryCalendarsInput {
  dateId: number; // 必填（YYYYMMDD）
  source?: CalendarSource;
  type?: CalendarType;
  category?: string;
  country?: string;
  minImportance?: number;
}


export const GetSourceText = (source: string) => {
  const sourceText = SourceMap[source];
  if (sourceText) {
    return sourceText;
  }
  return source;
};

export enum ExtractRuleType {
  unspecified = 'unspecified',
  xpath = 'xpath',
  regex = 'regex',
}

export interface Channel {
  id: string;
  name: string;
  title: string;
  broadcast: boolean;
  source: string;
  catalog: DocumentCatalog;
  extractCfg: ExtractCfg;
  enabled: boolean;
  createdAt: number;
  updatedAt: number;
}

export const ExtractCfgSchema = z.object({
  plans: z.nullish(
    z.array(
      z.object({
        seqNo: z.number().int(),
        matchRegex: z.string(),
        fields: z.array(
          z.object({
            key: z.string(),
            rule: z.object({
              type: z.enum([ExtractRuleType.regex, ExtractRuleType.xpath]),
              pattern: z.string(),
              group: z.number().int(),
            }),
            timeFormat: z.string().optional(),
          }),
        ),
      }),
    ),
  ),
  filterRegexs: z.nullish(z.array(z.string())),
});

export type ExtractCfg = z.infer<typeof ExtractCfgSchema>;

export interface ChannelsConnection {
  totalCount: number;
  list: Channel[];
}

export interface QueryChannelsParams {
  limit: number;
  offset: number;
  id?: string;
  name?: string;
  source?: string;
  catalog?: DocumentCatalog;
  enabled?: boolean;
}

export interface CreateChannelInput {
  id: string;
  name: string;
  title: string;
  broadcast: boolean;
  source: string;
  catalog: DocumentCatalog;
  extractCfg: ExtractCfg;
  enabled: boolean;
}

export interface UpdateChannelInput {
  id: string;
  name?: string;
  title?: string;
  source?: string;
  broadcast?: boolean;
  catalog?: DocumentCatalog;
  extractCfg?: ExtractCfg;
  enabled?: boolean;
}

export interface ExtractTestInput {
  extractCfg: ExtractCfg;
  text: string;
}

export interface ExtractTestResult {
  filtered: boolean;
  hitPlan?: number;
  title?: string;
  content?: string;
  url?: string;
  publishedAt?: number;
}

export const checkExtractCfg = (
  extractCfg: ExtractCfg,
): { valid: boolean; field: string; error: string } => {
  try {
    if (!extractCfg?.plans || extractCfg?.plans?.length === 0) {
      return { valid: true, field: '', error: '' };
    }

    if (extractCfg?.filterRegexs && extractCfg?.filterRegexs?.length > 0) {
      for (const [index, filterRegex] of extractCfg.filterRegexs.entries()) {
        try {
          new RegExp(filterRegex);
        } catch (e) {
          return { valid: false, field: `filterRegexs[${index}]`, error: 'invalid' };
        }
      }
    }

    const existingSeqNos = new Set<number>();
    for (const [index, plan] of extractCfg.plans?.entries() || []) {
      if (plan.seqNo <= 0) {
        return {
          valid: false,
          field: `plans[${index}].seqNo`,
          error: 'must be greater than 0',
        };
      }
      if (existingSeqNos.has(plan.seqNo)) {
        return { valid: false, field: `plans[${index}].seqNo`, error: 'must be unique' };
      }
      existingSeqNos.add(plan.seqNo);
      try {
        new RegExp(plan.matchRegex);
      } catch (e) {
        return {
          valid: false,
          field: `plans[seqNo=${plan.seqNo}].matchRegex`,
          error: 'invalid',
        };
      }
      if (plan.fields.length === 0) {
        return {
          valid: false,
          field: `plans[seqNo=${plan.seqNo}].fields`,
          error: 'must be non-empty',
        };
      }
      const existingKeys = new Set<string>();
      for (const [fieldIndex, field] of plan.fields.entries()) {
        if (field.key === '') {
          return {
            valid: false,
            field: `plans[seqNo=${plan.seqNo}].fields[${fieldIndex}].key`,
            error: 'must be non-empty',
          };
        }
        if (existingKeys.has(field.key)) {
          return {
            valid: false,
            field: `plans[seqNo=${plan.seqNo}].fields[${fieldIndex}].key`,
            error: 'must be unique',
          };
        }
        existingKeys.add(field.key);
        switch (field.rule.type) {
          case ExtractRuleType.regex:
            try {
              new RegExp(field.rule.pattern);
            } catch (e) {
              return {
                valid: false,
                field: `plans[seqNo=${plan.seqNo}].fields[key=${field.key}].rule.pattern`,
                error: 'invalid',
              };
            }
            if (
              field.rule.group == null ||
              field.rule.group == undefined ||
              field.rule.group <= 0
            ) {
              return {
                valid: false,
                field: `plans[seqNo=${plan.seqNo}].fields[key=${field.key}].rule.group`,
                error: 'must be greater than 0',
              };
            }
            break;
          case ExtractRuleType.xpath:
            try {
              document.evaluate(field.rule.pattern, document, null, XPathResult.ANY_TYPE, null);
            } catch (e) {
              return {
                valid: false,
                field: `plans[seqNo=${plan.seqNo}].fields[key=${field.key}].rule.pattern`,
                error: 'invalid',
              };
            }
            break;
          default:
            return {
              valid: false,
              field: `plans[seqNo=${plan.seqNo}].fields[key=${field.key}].rule.type`,
              error: 'invalid',
            };
        }
      }
    }
    return { valid: true, field: '', error: '' };
  } catch (e) {
    console.error(e);
    return { valid: false, field: '', error: 'Invalid JSON format' };
  }
};

export const parseExtractCfgJson = (
  json: string,
): { valid: boolean; field: string; error: string; data?: ExtractCfg } => {
  try {
    const parsed = JSON.parse(json);

    const parsedResult = ExtractCfgSchema.safeParse(parsed);
    if (!parsedResult.success) {
      return { valid: false, field: '', error: parsedResult.error.message };
    }

    const extractCfg: ExtractCfg = parsedResult.data;
    const checkResult = checkExtractCfg(extractCfg);
    if (!checkResult.valid) {
      return checkResult;
    }
    return { valid: true, field: '', error: '', data: extractCfg };
  } catch (e) {
    console.error(e);
    return { valid: false, field: '', error: 'Invalid JSON format' };
  }
};


const QUERY_DOCUMENTS_SCHEMA = `
    query Documents($input: QueryDocumentsInput!) {
      Result: Documents(input: $input) {
        totalCount
        list {
          id
          source
          provider
          catalog
          title
          content
          aiTitle
          aiSummary
          aiTags
          aiCoins
          aiInfluence
          aiInfluenceScore
          aiSentiment
          lang
          md5
          url
          authors
          format
          status
          errMsg
          dedupedBy
          publishedAt
          createdAt
          updatedAt
        }
      }
    }
  `;

// GraphQL 查询文档列表
export async function queryDocuments(params: QueryDocumentParams) {
  const pageSize = params.pageSize || 10;
  const input: any = {
    limit: pageSize,
    offset: ((params?.current || 1) - 1) * pageSize,
    id: params.id ? params.id : undefined,
    publishedAtStart: params.publishedAtRange
      ? dayjs(params.publishedAtRange[0]).unix()
      : undefined,
    publishedAtEnd: params.publishedAtRange ? dayjs(params.publishedAtRange[1]).unix() : undefined,
    catalog: params.catalog,
    status: params.status,
    source: params.source ? params.source : undefined,
    provider: params.provider ? params.provider : undefined,
    keyword: params.keyword ? params.keyword : undefined,
    tag: params.tag ? params.tag : undefined,
    coin: params.coin ? params.coin : undefined,
    influenceScore: params.influenceScore ? params.influenceScore : undefined,
    sentiment: params.sentiment ? params.sentiment : undefined,
  };

  let response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_DOCUMENTS_SCHEMA,
      variables: {
        input: input,
      },
    }),
  });
  return response.data?.Result;
}

const GET_DOCUMENT_SCHEMA = `
  query GetDocument($input: GetDocumentInput!) {
    Result: Document(input: $input) {
      id
      source
      provider
      catalog
      title
      content
      aiTitle
      aiSummary
      aiTags
      aiCoins
      aiInfluence
      aiInfluenceScore
      aiSentiment
      lang
      md5
      url
      authors
      format
      status
      errMsg
      dedupedBy
      publishedAt
      createdAt
      updatedAt
    }
  }
`;

export async function getDocument(id: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: GET_DOCUMENT_SCHEMA,
      variables: {
        input: { id: id },
      },
    }),
  });
  return response.data?.Result;
}

const ARCHIVE_DOCUMENT_SCHEMA = `
  mutation ArchiveDocument($input: ArchiveDocumentInput!) {
    Result: ArchiveDocument(input: $input)
  }
`;

export async function archiveDocument(id: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: ARCHIVE_DOCUMENT_SCHEMA,
      variables: {
        input: { id: id },
      },
    }),
  });
  return response.data?.Result;
}

const DOCUMENT_SIMILARITY_SCHEMA = `
  query DocumentSimilarity($input: DocumentSimilarityInput!) {
    Result: DocumentSimilarity(input: $input) {
      leftId
      rightId
      similarity
    }
  }
`;

export async function getDocumentSimilarity(
  leftId: string,
  rightId: string,
): Promise<DocumentSimilarityResult | undefined> {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: DOCUMENT_SIMILARITY_SCHEMA,
      variables: {
        input: { leftId, rightId },
      },
    }),
  });
  return response.data?.Result;
}


const QUERY_CHANNELS_SCHEMA = `
  query QueryChannels($input: QueryChannelsInput!) {
    Channels(input: $input) {
      totalCount
      list {
        id
        name
        title
        broadcast
        source
        catalog
        extractCfg {
          filterRegexs
          plans {
            seqNo
            matchRegex
            fields {
              key
              rule {
                type
                pattern
                group
              }
              timeFormat
            }
          }
        }
        enabled
        createdAt
        updatedAt
      }
    }
  }
`;

const CREATE_CHANNEL_SCHEMA = `
  mutation CreateChannel($input: CreateChannelInput!) {
    CreateChannel(input: $input) {
      id
      name
      title
      broadcast
      source
      catalog
      extractCfg {
        filterRegexs
        plans {
          seqNo
          matchRegex
          fields {
            key
            rule {
              type
              pattern
              group
            }
            timeFormat
          }
        }
      }
      enabled
      createdAt
      updatedAt
    }
  }
`;

const UPDATE_CHANNEL_SCHEMA = `
  mutation UpdateChannel($input: UpdateChannelInput!) {
    UpdateChannel(input: $input) {
      id
      name
      title
      broadcast
      source
      catalog
      extractCfg {
        filterRegexs
        plans {
          seqNo
          matchRegex
          fields {
            key
            rule {
              type
              pattern
              group
            }
            timeFormat
          }
        }
      }
      enabled
      createdAt
      updatedAt
    }
  }
`;

const TEST_EXTRACT_SCHEMA = `
  mutation TestExtract($input: TestExtractInput!) {
    TestExtract(input: $input) {
      filtered
      hitPlan
      title
      content
      url
      publishedAt
    }
  }
`;

export async function queryChannels(params: QueryChannelsParams): Promise<ChannelsConnection> {
  const response: any = await request('/query', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: {
      query: QUERY_CHANNELS_SCHEMA,
      variables: {
        input: params,
      },
    },
  });

  if (response?.errors) {
    throw new Error(response.errors[0]?.message || 'Failed to query Channels');
  }

  return response?.data?.Channels;
}

export async function createChannel(input: CreateChannelInput): Promise<Channel> {
  const response: any = await request('/query', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: {
      query: CREATE_CHANNEL_SCHEMA,
      variables: {
        input,
      },
    },
  });

  if (response?.errors) {
    throw new Error(response.errors[0]?.message || 'Failed to create Channel');
  }

  return response?.data?.CreateChannel;
}

export async function updateChannel(input: UpdateChannelInput): Promise<Channel> {
  const response: any = await request('/query', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: {
      query: UPDATE_CHANNEL_SCHEMA,
      variables: {
        input,
      },
    },
  });

  if (response?.errors) {
    throw new Error(response.errors[0]?.message || 'Failed to update Channel');
  }

  return response?.data?.UpdateChannel;
}

export async function testExtract(input: ExtractTestInput): Promise<ExtractTestResult> {
  const response: any = await request('/query', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: {
      query: TEST_EXTRACT_SCHEMA,
      variables: {
        input,
      },
    },
  });

  if (response?.errors) {
    throw new Error(response.errors[0]?.message || 'Failed to test extract');
  }

  return response?.data?.TestExtract;
}


const QUERY_CALENDARS_SCHEMA = `
  query Calendars($input: QueryCalendarsInput!) {
    Result: Calendars(input: $input) {
      id
      dateId
      source
      sid
      type
      category
      country
      project
      symbol
      title
      content
      importance
      url
      ext {
        __typename
        ... on EconomicCalendarExtention {
          unit
          actual
          previous
          consensus
        }
      }
      publishedAt
      createdAt
      updatedAt
    }
  }
`;

export interface QueryCalendarsParams {
  date?: string | number | Date; // 日期，默认今天
  source?: string;
  type?: string;
  category?: string;
  country?: string;
  minImportance?: number;
}

export async function queryCalendars(params: QueryCalendarsParams): Promise<CalendarItem[]> {
  const targetDate = params?.date ? dayjs(params.date) : dayjs();
  const input: QueryCalendarsInput = {
    dateId: parseInt(targetDate.format('YYYYMMDD'), 10),
    source: params.source as any,
    type: params.type as any,
    category: params.category,
    country: params.country,
    minImportance: params.minImportance,
  };

  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: QUERY_CALENDARS_SCHEMA,
      variables: { input },
    }),
  });

  return response?.data?.Result || [];
}

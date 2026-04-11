// @ts-ignore
/* eslint-disable */
import { request } from '@umijs/max';

// GraphQL 查询定义
export const HOT_TOKENS_QUERY = `
  query HotTokens($limit: Int!) {
    HotTokens(limit: $limit) {
      id
      address
      symbol
      decimals
      name
      network
      description
      logo
      creatorId
      socialMedia {
        website
        twitter
        telegram
      }
      extra {
        bondingCurve
        associatedBondingCurve
        raydiumPool
        complete
        virtualSolReserves
        virtualTokenReserves
        hidden
        totalSupply
        showName
        lastTradeTimestamp
        kingOfTheHillTimestamp
        marketCap
        nsfw
        marketId
        inverted
        realSolReserves
        realTokenReserves
        livestreamBanExpiry
        lastReply
        replyCount
        isBanned
        isCurrentlyLive
        initialized
        updatedAtStr
        pumpSwapPool
        athMarketCap
        athMarketCapTimestamp
        bannerUri
        hideBanner
        livestreamDownrankScore
        usdMarketCap
        creatorUsername
      }
      logoObjectName
      groupId
      score
      heat
      scoreUpdatedAt
      releaseAt
      createdAt
      updatedAt
    }
  }
`;

export const HOT_CREATORS_QUERY = `
  query HotCreators($limit: Int!) {
    HotCreators(limit: $limit) {
      id
      address
      network
      username
      avatar
      description
      xId
      score
      heat
      scoreUpdatedAt
      stats {
        Followers
        Followings
        LikesReceived
        MentionsReceived
        CreatedCoinsCount
      }
      extra
      createdAt
      updatedAt
    }
  }
`;

export const HOT_TRADERS_QUERY = `
  query HotTraders($limit: Int!) {
    HotTraders(limit: $limit) {
      id
      address
      network
      score
      heat
      scoreUpdatedAt
      createdAt
      updatedAt
    }
  }
`;

export const HOT_GROUPS_QUERY = `
  query HotGroups($limit: Int!) {
    HotGroups(limit: $limit) {
      id
      name
      description
      score
      heat
      scoreUpdatedAt
      createdAt
      updatedAt
    }
  }
`;

export const SURGE_TOKENS_QUERY = `
  query SurgeTokens($limit: Int!, $timeRange: TimeRange!) {
    SurgeTokens(limit: $limit, timeRange: $timeRange) {
      id
      address
      symbol
      decimals
      name
      network
      description
      logo
      creatorId
      socialMedia {
        website
        twitter
        telegram
      }
      extra {
        bondingCurve
        associatedBondingCurve
        raydiumPool
        complete
        virtualSolReserves
        virtualTokenReserves
        hidden
        totalSupply
        showName
        lastTradeTimestamp
        kingOfTheHillTimestamp
        marketCap
        nsfw
        marketId
        inverted
        realSolReserves
        realTokenReserves
        livestreamBanExpiry
        lastReply
        replyCount
        isBanned
        isCurrentlyLive
        initialized
        updatedAtStr
        pumpSwapPool
        athMarketCap
        athMarketCapTimestamp
        bannerUri
        hideBanner
        livestreamDownrankScore
        usdMarketCap
        creatorUsername
      }
      logoObjectName
      groupId
      score
      heat
      scoreUpdatedAt
      releaseAt
      createdAt
      updatedAt
    }
  }
`;

export const SURGE_GROUPS_QUERY = `
  query SurgeGroups($limit: Int!, $timeRange: TimeRange!) {
    SurgeGroups(limit: $limit, timeRange: $timeRange) {
      id
      name
      description
      score
      heat
      scoreUpdatedAt
      createdAt
      updatedAt
    }
  }
`;

// API 函数
export async function getHotTokens(limit: number) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: HOT_TOKENS_QUERY,
      variables: { limit },
    }),
  });
  return response?.data?.HotTokens || [];
}

export async function getHotCreators(limit: number) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: HOT_CREATORS_QUERY,
      variables: { limit },
    }),
  });
  return response?.data?.HotCreators || [];
}

export async function getHotTraders(limit: number) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: HOT_TRADERS_QUERY,
      variables: { limit },
    }),
  });
  return response?.data?.HotTraders || [];
}

export async function getHotGroups(limit: number) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: HOT_GROUPS_QUERY,
      variables: { limit },
    }),
  });
  return response?.data?.HotGroups || [];
}

export async function getSurgeTokens(limit: number, timeRange: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: SURGE_TOKENS_QUERY,
      variables: { limit, timeRange },
    }),
  });
  return response?.data?.SurgeTokens || [];
}

export async function getSurgeGroups(limit: number, timeRange: string) {
  const response = await request('/query', {
    method: 'POST',
    data: JSON.stringify({
      query: SURGE_GROUPS_QUERY,
      variables: { limit, timeRange },
    }),
  });
  return response?.data?.SurgeGroups || [];
}

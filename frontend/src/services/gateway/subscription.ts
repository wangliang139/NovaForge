import { gql } from '@apollo/client';

const STREAM_POSITION_FIELDS = `
  symbol
  side
  isolated
  amount
  entryPrice
  markPrice
  liquidationPrice
  notional
  leverage
  initialMargin
  maintMargin
  unRealizedProfit
  updatedTs
`;

const STREAM_ORDER_FIELDS = `
  accountId
  botId
  exchange
  symbol
  clientOrderId
  orderId
  drivedOrderId
  side
  isBuy
  orderType
  algoType
  source
  price
  originalQty
  executedQty
  originalQuoteQty
  executedQuoteQty
  avgPrice
  priceWorkingType
  priceMode
  status
  timeInForce
  reduceOnly
  closePosition
  postOnly
  priceProtect
  conditions {
    triggerType
    orderPrice
    callbackDistance
    callbackRate
    activationPrice
    priceWorkingType
    priceMode
    isTrailing
    activated
    activatedTs
  }
  isWorking
  workingTs
  rejectReason
  createdTs
  updatedTs
  finishedTs
  locked
  lockedAsset
  fee
  feeAsset
  realizedPnl
  pnlAsset
`;

export const SUB_STREAM = gql`
  subscription Stream($input: StreamInput!) {
    Stream(input: $input) {
      type
      eventTs
      ticker {
        exchange
        symbol
        lastPrice
        open24H
        high24H
        low24H
        avg24H
        volume24H
        quoteVolume24H
        ts
      }
      trade {
        tradeId
        exchange
        symbol
        price
        size
        isBuy
        ts
      }
      depth {
        ts
        bids {
          price
          size
        }
        asks {
          price
          size
        }
        seqId
        prevSeqId
      }
      kline {
        interval
        open
        high
        low
        close
        volume
        quoteVolume
        trades
        openTs
        closeTs
      }
      markPrice {
        exchange
        symbol
        markPrice
        ts
      }
      social {
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
      balanceSnapshot {
        scope
        assets {
          walletType
          code
          balance
          locked
          updatedTs
        }
      }
      balanceUpdate {
        eventId
        type
        reason
        assets {
          walletType
          code
          balance
          locked
          updatedTs
        }
      }
      positionSnapshot {
        positions {
          ${STREAM_POSITION_FIELDS}
        }
      }
      positionsUpdate {
        eventId
        type
        reason
        positions {
          ${STREAM_POSITION_FIELDS}
        }
      }
      order {
        ${STREAM_ORDER_FIELDS}
      }
      fill {
        exchange
        symbol
        orderId
        clientOrderId
        tradeId
        side
        isBuy
        qty
        price
        fee
        feeAsset
        realizedPnl
        isMaker
        ts
      }
      symbolLeverage {
        exchange
        symbol
        side
        leverage
        updatedTs
      }
    }
  }
`;

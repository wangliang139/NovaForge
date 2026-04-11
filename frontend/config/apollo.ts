import { ApolloClient, InMemoryCache, HttpLink, ApolloLink, split } from '@apollo/client';
import { GraphQLWsLink } from '@apollo/client/link/subscriptions';
import { getMainDefinition } from '@apollo/client/utilities';
import { setContext } from '@apollo/client/link/context';
import { onError } from '@apollo/client/link/error';
// @ts-ignore
import createUploadLink from 'apollo-upload-client/createUploadLink.mjs';
import { createClient } from 'graphql-ws';
import { getAccessToken } from '../src/utils/auth';

const endpoint = API_URL + '/query';
const wsEndpoint = endpoint.replace(/^http/i, 'ws');

const isFile = (value: any) =>
  (typeof File !== 'undefined' && value instanceof File) ||
  (typeof Blob !== 'undefined' && value instanceof Blob);
const isUpload = ({ variables }: { variables: object }) => Object.values(variables).some(isFile);

// Auth link: 为所有请求注入 Authorization header
const authLink = setContext((_, { headers }) => {
  const token = getAccessToken();
  return {
    headers: {
      ...headers,
      ...(token ? { authorization: `Bearer ${token}` } : {}),
    },
  };
});

const errorLink = onError(({ graphQLErrors, networkError }) => {
  if (graphQLErrors) {
    graphQLErrors.forEach(({ message, locations, path }) => {
      console.log(`[GraphQL error]: Message: ${message}, Location: ${locations}, Path: ${path}`);
    });
  }

  if (networkError) {
    console.log(`[Network error]: ${networkError}`);
  }
});

const httpLink = new HttpLink({
  uri: endpoint,
  credentials: 'same-origin',
});

const requestLink = ApolloLink.from([errorLink, authLink, httpLink]);

// Upload link 也需要 auth
const uploadLink = ApolloLink.from([
  errorLink,
  authLink,
  createUploadLink({ uri: endpoint }),
]);

const wsLink =
  typeof window !== 'undefined'
    ? new GraphQLWsLink(
        createClient({
          url: wsEndpoint,
          connectionParams: () => {
            const token = getAccessToken();
            if (!token) {
              return {};
            }
            return {
              Authorization: `Bearer ${token}`,
            };
          },
          retryAttempts: 100,
          retryWait: async (retries) => {
            const delay = Math.min(1000 * 2 ** retries, 30_000);
            await new Promise((r) => setTimeout(r, delay));
          },
        }),
      )
    : null;

const isSubscription = ({ query }: { query: any }) => {
  const definition = getMainDefinition(query);
  return definition.kind === 'OperationDefinition' && definition.operation === 'subscription';
};

const client = new ApolloClient({
  cache: new InMemoryCache(),
  link:
    wsLink != null
      ? split(isSubscription, wsLink, split(isUpload, uploadLink, requestLink))
      : split(isUpload, uploadLink, requestLink),
});

export default client;

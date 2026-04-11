import useStyles from '@/pages/document/documents/style.style';
import {
  archiveDocument,
  Document,
  DocumentCatalog,
  DocumentStatus,
  getDocument, getDocumentSimilarity,
  GetSourceText,
  QueryDocumentParams,
  queryDocuments,
} from '@/services/gateway/document';
import { enumToOptions } from '@/utils/dict';
import { DiffOutlined, InfoCircleOutlined, OpenAIOutlined } from '@ant-design/icons';
import {
  ActionType,
  PageContainer,
  ProColumns,
  ProDescriptions,
  ProDescriptionsItemProps,
  ProFormInstance,
  ProTable,
} from '@ant-design/pro-components';
import {
  Button,
  Card,
  Dropdown,
  Flex,
  Input,
  message,
  Modal,
  Row,
  Space,
  Splitter,
  Tag,
  theme,
  Tooltip,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import DOMPurify from 'dompurify';
import { MenuInfo } from 'rc-menu/lib/interface';
import React, { useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeHighlight from 'rehype-highlight';
import remarkGfm from 'remark-gfm';

DOMPurify.addHook('uponSanitizeElement', (node, data) => {
  if (data.tagName === 'img' || data.tagName === 'video') {
    (node as HTMLElement).removeAttribute('width');
    (node as HTMLElement).removeAttribute('height');
  } else if (data.tagName === 'a' || data.tagName === 'A') {
    (node as HTMLLinkElement).setAttribute('target', '_blank');
    (node as HTMLLinkElement).setAttribute('rel', 'noopener noreferrer');
  }
});

const { TextArea } = Input;

const getCatalogColor = (catalog: DocumentCatalog) => {
  switch (catalog) {
    case DocumentCatalog.AIRDROP:
      return 'green';
    case DocumentCatalog.NEWS:
      return 'red';
    case DocumentCatalog.API:
      return 'purple';
    case DocumentCatalog.CRYPTOCURRENCY_LISTING:
      return 'cyan';
    case DocumentCatalog.CRYPTOCURRENCY_DELISTING:
      return 'orange';
    case DocumentCatalog.ACTIVITY:
      return 'magenta';
    case DocumentCatalog.FLASH_NEWS:
      return 'yellow';
    case DocumentCatalog.OTHER:
      return 'gray';
    default:
      return 'default';
  }
};

const getStatusColor = (status: DocumentStatus) => {
  switch (status) {
    case DocumentStatus.DRAFT:
      return 'blue';
    case DocumentStatus.DRAFT_FAILED:
      return 'red';
    case DocumentStatus.PENDING:
      return 'orange';
    case DocumentStatus.PENDING_FAILED:
      return 'red';
    case DocumentStatus.ACTIVE:
      return 'green';
    case DocumentStatus.ARCHIVED:
      return 'gray';
    case DocumentStatus.DEDUPED:
      return 'purple';
    default:
      return 'default';
  }
};

const DocumentList: React.FC = () => {
  const { styles } = useStyles();
  const { token } = theme.useToken();

  const actionRef = useRef<ActionType>();
  const formRef = useRef<ProFormInstance>();
  const [showDetail, setShowDetail] = useState<boolean>(false);
  const [currentRow, setCurrentRow] = useState<Document>();

  const [showCompare, setShowCompare] = useState<boolean>(false);
  const [compareDoc1, setCompareDoc1] = useState<Document>();
  const [compareDoc2, setCompareDoc2] = useState<Document>();

  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
  const [selectedRows, setSelectedRows] = useState<Document[]>([]);
  const [manualCompareLoading, setManualCompareLoading] = useState<boolean>(false);
  const [similarityScore, setSimilarityScore] = useState<number>();
  const [similarityLoading, setSimilarityLoading] = useState<boolean>(false);

  const ref1 = useRef(null);
  const ref2 = useRef(null);
  const [height, setHeight] = useState<number>(200);

  const updateHeight = () => {
    if (ref1.current && ref2.current) {
      const h = Math.max(
        (ref1.current as HTMLElement).offsetHeight,
        (ref2.current as HTMLElement).offsetHeight,
      );
      setHeight(h);
    }
  };

  const handleAfterOpen = (isOpen: boolean) => {
    if (isOpen) {
      setTimeout(updateHeight); // 保险：延迟 0ms 等 DOM 完整渲染
    } else {
      setHeight(200); // 关闭时重置高度
    }
  };

  const openCompareModal = (doc1: Document, doc2: Document) => {
    setCompareDoc1(doc1);
    setCompareDoc2(doc2);
    setSimilarityScore(undefined);
    setShowCompare(true);
  };

  const handleCloseCompare = () => {
    setShowCompare(false);
    setCompareDoc1(undefined);
    setCompareDoc2(undefined);
    setSimilarityScore(undefined);
    setSimilarityLoading(false);
  };

  const handleManualCompare = async () => {
    if (selectedRows.length !== 2) {
      message.warning('请选择两条文档进行比较');
      return;
    }
    setManualCompareLoading(true);
    try {
      const [first, second] = selectedRows;
      const [doc1, doc2] = await Promise.all([getDocument(first.id), getDocument(second.id)]);
      if (!doc1 || !doc2) {
        message.error('无法获取所选文档信息');
        return;
      }
      openCompareModal(doc1, doc2);
      // setSelectedRowKeys([]);
      // setSelectedRows([]);
    } catch (error) {
      message.error('获取文档信息失败');
    } finally {
      setManualCompareLoading(false);
    }
  };

  const handleSimilarityCompare = async () => {
    if (!compareDoc1 || !compareDoc2) {
      return;
    }
    setSimilarityLoading(true);
    try {
      const result = await getDocumentSimilarity(compareDoc1.id, compareDoc2.id);
      if (!result) {
        message.error('未获取到语义相似度结果');
        return;
      }
      setSimilarityScore(result.similarity);
    } catch (error) {
      message.error('计算语义相似度失败');
    } finally {
      setSimilarityLoading(false);
    }
  };

  const handleSelectionChange = (keys: React.Key[], rows: Document[]) => {
    setSelectedRowKeys(keys);
    setSelectedRows(rows);
  };

  const clearSelection = () => {
    setSelectedRowKeys([]);
    setSelectedRows([]);
  };

  const handleMenuClick = async (e: MenuInfo, row: Document) => {
    if (e.key === 'original') {
      window.open(row.url, '_blank');
    }
    if (e.key === 'archive') {
      const resp = await archiveDocument(row.id);
      if (!resp.errors) {
        message.success(`Document archived successfully!`);
        setShowDetail(false);
        actionRef.current?.reload();
      }
    }
  };

  const handleCompareClick = async (row: Document, dedupedById: string) => {
    try {
      const doc1 = row;
      const doc2 = await getDocument(dedupedById);
      if (doc2) {
        openCompareModal(doc1, doc2);
      } else {
        message.error('无法获取被去重的文档信息');
      }
    } catch (error) {
      message.error('获取文档信息失败');
    }
  };

  const renderMenus = (row: Document) => {
    let menus = [];

    if (row.url) {
      menus.push({ key: 'original', label: '原文', onClick: () => window.open(row.url, '_blank') });
    }

    if (row.status === DocumentStatus.ACTIVE || row.status === DocumentStatus.PENDING) {
      menus.push({ key: 'archive', label: '归档', danger: true });
    }

    return [
      <Dropdown.Button
        type="primary"
        key="view"
        arrow={true}
        menu={{
          items: menus,
          onClick: async (e: MenuInfo) => {
            await handleMenuClick(e, row);
          },
        }}
        onClick={() => {
          setCurrentRow(row);
          setShowDetail(true);
        }}
        children={'查看'}
      />,
    ];
  };

  const renderText = (text: string, format: string) => {
    if (format === 'markdown') {
      return (
        <Row>
          <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
            {text}
          </ReactMarkdown>
        </Row>
      );
    }
    // if (row?.format === 'html') {
    return (
      <div
        className={styles.htmlContent}
        dangerouslySetInnerHTML={{
          __html: DOMPurify.sanitize(text, {
            ADD_ATTR: ['target', 'rel', 'href'],
            ALLOWED_ATTR: ['target', 'rel', 'href'],
          }),
        }}
      />
    );
    // }
    // return <TextArea style={{ width: '100%' }} autoSize value={row.content} />;
  };

  const columns: ProColumns<Document>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      width: 120,
      order: 11,
      // hideInSearch: true,
      hideInDescriptions: true,
      fieldProps: {
        onChange: (e) => {
          let value = '';
          let num = parseFloat(e.target.value.trim());
          if (Number.isInteger(num)) {
            value = num.toString();
          } else if (!isNaN(num)) {
            value = Math.floor(num).toString();
          }
          formRef.current?.setFieldsValue({
            id: value,
          });
        },
      },
      render: (dom, entity) => {
        return (
          <a
            onClick={() => {
              setCurrentRow(entity);
              setShowDetail(true);
            }}
          >
            {dom}
          </a>
        );
      },
    },
    {
      title: 'Keyword',
      dataIndex: 'keyword',
      order: 10,
      hideInTable: true,
      hideInForm: true,
      hideInDescriptions: true,
      ellipsis: true,
    },
    {
      title: '标签',
      dataIndex: 'tag',
      order: 6,
      hideInForm: true,
      hideInTable: true,
      hideInDescriptions: true,
    },
    {
      title: '币种',
      dataIndex: 'coin',
      order: 7,
      hideInForm: true,
      hideInTable: true,
      hideInDescriptions: true,
    },
    {
      title: '标题',
      dataIndex: 'title',
      hideInDescriptions: true,
      ellipsis: true,
      hideInSearch: true,
    },
    {
      title: '来源',
      align: 'center',
      dataIndex: 'source',
      width: 120,
      order: 8,
      render: (dom, row) => {
        return <Tag>{GetSourceText(row.source)}</Tag>;
      },
    },
    {
      title: 'Provider',
      dataIndex: 'provider',
      align: 'center',
      width: 120,
      render: (dom, row) => {
        if (row.provider.length > 12) {
          return (
            <Tooltip title={row.provider}>
              <Tag>{row.provider.slice(0, 12)}...</Tag>
            </Tooltip>
          );
        }
        return <Tag>{row.provider}</Tag>;
      },
    },
    {
      title: '作者',
      dataIndex: 'authors',
      hideInTable: true,
      hideInForm: true,
      hideInSearch: true,
      ellipsis: true,
      render: (dom, row) => {
        if (!row?.authors || row?.authors?.length === 0) {
          return '--';
        }
        return (
          <Space size={0}>
            {row.authors.map((author) => (
              <Tag key={author}>{author}</Tag>
            ))}
          </Space>
        );
      },
    },
    {
      title: '分类',
      dataIndex: 'catalog',
      valueType: 'select',
      align: 'center',
      width: 120,
      fieldProps: {
        options: enumToOptions(DocumentCatalog, 'UNSPECIFIED'),
      },
      render: (dom, row) => {
        return <Tag color={getCatalogColor(row.catalog)}>{row.catalog}</Tag>;
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      valueType: 'select',
      align: 'center',
      width: 100,
      order: 9,
      initialValue: DocumentStatus.ACTIVE,
      hideInDescriptions: true,
      fieldProps: {
        options: enumToOptions(DocumentStatus, 'UNSPECIFIED'),
      },
      render: (dom, row) => {
        return <Tag color={getStatusColor(row.status)}>{row.status}</Tag>;
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      hideInSearch: true,
      hideInForm: true,
      hideInTable: true,
      render: (dom, row) => {
        return (
          <Flex>
            <Tag color={getStatusColor(row.status)}>{row.status}</Tag>
            {row.errMsg && row.status !== DocumentStatus.ACTIVE && (
              <Tooltip style={{ marginLeft: 8 }} title={row.errMsg || ''}>
                <InfoCircleOutlined style={{ color: token.colorError, fontSize: 16 }} />
              </Tooltip>
            )}
          </Flex>
        );
      },
    },
    {
      title: '被去重',
      dataIndex: 'dedupedBy',
      width: 100,
      hideInSearch: true,
      hideInForm: true,
      hideInTable: true,
      render: (dom, row) => {
        if (!row.dedupedBy) {
          return '--';
        }
        return (
          <Space>
            <a
              onClick={() => {
                navigator.clipboard.writeText(row.dedupedBy.toString());
                message.success('已复制');
              }}
            >
              {row.dedupedBy}
            </a>
            <DiffOutlined
              style={{ color: token.colorPrimary, cursor: 'pointer' }}
              onClick={() => handleCompareClick(row, row.dedupedBy)}
            />
          </Space>
        );
      },
    },
    {
      title: '语种',
      dataIndex: 'lang',
      hideInSearch: true,
      hideInTable: true,
      render: (dom) => {
        return <Tag>{dom}</Tag>;
      },
    },
    {
      title: '格式',
      dataIndex: 'format',
      hideInSearch: true,
      hideInTable: true,
      render: (dom) => {
        return <Tag>{dom}</Tag>;
      },
    },
    {
      title: '发布时间',
      dataIndex: 'publishedAtRange',
      valueType: 'dateTimeRange',
      colSize: 2,
      ellipsis: true,
      hideInTable: true,
      hideInForm: true,
      hideInDescriptions: true,
      fieldProps: {
        allowClear: true,
      },
    },
    {
      title: '发布时间',
      dataIndex: 'publishedAt',
      align: 'center',
      width: 180,
      ellipsis: true,
      hideInSearch: true,
      renderText: (text) => (text >= 0 ? dayjs.unix(text).format('YYYY-MM-DD HH:mm:ss') : ''),
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      ellipsis: true,
      hideInSearch: true,
      hideInTable: true,
      renderText: (text) => (text >= 0 ? dayjs.unix(text).format('YYYY-MM-DD HH:mm:ss') : ''),
    },
    {
      title: '更新时间',
      dataIndex: 'updatedAt',
      ellipsis: true,
      hideInSearch: true,
      hideInTable: true,
      renderText: (text) => (text >= 0 ? dayjs.unix(text).format('YYYY-MM-DD HH:mm:ss') : ''),
    },
    {
      title: '操作',
      valueType: 'option',
      align: 'center',
      hideInDescriptions: true,
      width: 120,
      render: (dom, row) => {
        return renderMenus(row);
      },
    },
  ];

  return (
    <PageContainer>
      <ProTable<Document, QueryDocumentParams>
        actionRef={actionRef}
        formRef={formRef}
        form={{ span: 6 }}
        headerTitle="文档列表"
        rowKey={(record) => record.id}
        search={{
          labelWidth: 'auto',
          showHiddenNum: true,
        }}
        request={async (params: QueryDocumentParams) => {
          const res = await queryDocuments(params);
          return {
            data: res.list,
            total: res.totalCount,
            success: true,
          };
        }}
        pagination={{
          showSizeChanger: true,
        }}
        columns={columns}
        dateFormatter={'number'}
        rowSelection={{
          type: 'checkbox',
          selectedRowKeys,
          onChange: (keys, rows) => handleSelectionChange(keys, rows),
        }}
        tableAlertRender={({ selectedRowKeys }) => <span>已选 {selectedRowKeys.length} 项</span>}
        tableAlertOptionRender={({ onCleanSelected }) => (
          <a
            onClick={() => {
              onCleanSelected?.();
              clearSelection();
            }}
          >
            清空
          </a>
        )}
        toolBarRender={() => [
          <Button
            key="compare"
            type="primary"
            disabled={selectedRows.length !== 2}
            onClick={handleManualCompare}
            loading={manualCompareLoading}
            icon={<DiffOutlined />}
          >
            比较
          </Button>,
        ]}
      />

      <Modal
        width={1000}
        open={showDetail}
        title={currentRow?.id}
        onCancel={() => {
          setCurrentRow(undefined);
          setShowDetail(false);
        }}
        afterOpenChange={handleAfterOpen}
        maskClosable={true}
        closable={true}
        footer={null}
        forceRender={true}
      >
        {currentRow?.title && (
          <>
            <ProDescriptions<Document>
              column={2}
              title={'ID: ' + currentRow?.id}
              dataSource={currentRow}
              columns={columns as ProDescriptionsItemProps<Document>[]}
              extra={renderMenus(currentRow)}
            />
            <Card
              className={styles.contentCard}
              title={
                <>
                  <OpenAIOutlined /> 文章内容
                </>
              }
              style={{ marginTop: 20, padding: 0 }}
            >
              <Splitter style={{ height: height + 5 }}>
                <Splitter.Panel resizable={false} collapsible>
                  <Typography
                    ref={ref1}
                    style={{
                      margin: 2,
                      padding: '16px 20px',
                      backgroundColor: token.colorFillAlter,
                      borderRadius: 8,
                      minHeight: height,
                    }}
                  >
                    {currentRow?.content ? (
                      <>
                        <Typography.Title level={4} style={{ marginBottom: 14 }}>
                          <Tooltip title={currentRow?.title}>{currentRow?.title}</Tooltip>
                        </Typography.Title>
                        <Typography.Text>
                          {renderText(currentRow.content, currentRow.format)}
                        </Typography.Text>
                      </>
                    ) : (
                      <>
                        <Typography.Text>
                          {renderText(currentRow.title, currentRow.format)}
                        </Typography.Text>
                      </>
                    )}
                  </Typography>
                </Splitter.Panel>
                <Splitter.Panel collapsible defaultSize={currentRow?.aiTitle ? '50%' : '0%'}>
                  <Typography
                    hidden={!currentRow?.aiTitle}
                    ref={ref2}
                    style={{
                      margin: 2,
                      padding: '16px 20px',
                      backgroundColor: token.colorFillAlter,
                      borderRadius: 8,
                      minHeight: height,
                    }}
                  >
                    <Typography.Title level={4} style={{ marginBottom: 14 }}>
                      <Tooltip title={currentRow?.aiTitle}>{currentRow?.aiTitle}</Tooltip>
                    </Typography.Title>
                    <Space direction="vertical" size={12} style={{ width: '100%' }}>
                      <div>
                        <Typography.Text strong style={{ marginRight: 8 }}>
                          标签:
                        </Typography.Text>
                        <Space size={4} wrap>
                          {currentRow.aiTags.map((tag, index) => (
                            <Tag key={index} color="blue">
                              {tag}
                            </Tag>
                          ))}
                        </Space>
                      </div>
                      <div>
                        <Typography.Text strong style={{ marginRight: 8 }}>
                          币种:
                        </Typography.Text>
                        <Space size={4} wrap>
                          {currentRow.aiCoins.map((coin, index) => (
                            <Tag key={index} color="gold">
                              {coin}
                            </Tag>
                          ))}
                        </Space>
                      </div>
                      <div>
                        <Typography.Text strong style={{ marginRight: 8 }}>
                          潜在影响:
                        </Typography.Text>
                        {currentRow.aiInfluence || '--'}
                      </div>
                      <div>
                        <Typography.Text strong style={{ marginRight: 8 }}>
                          影响力评分:
                        </Typography.Text>
                        <Tag color="cyan">{currentRow.aiInfluenceScore}</Tag>
                      </div>
                      <div>
                        <Typography.Text strong style={{ marginRight: 8 }}>
                          情感倾向:
                        </Typography.Text>
                        <Tag
                          color={
                            currentRow.aiSentiment > 0
                              ? 'green'
                              : currentRow.aiSentiment < 0
                                ? 'red'
                                : 'default'
                          }
                        >
                          {currentRow.aiSentiment}
                        </Tag>
                      </div>
                      <div>
                        <Typography.Text strong style={{ marginRight: 8 }}>
                          摘要:
                        </Typography.Text>
                        <Typography.Text>{currentRow?.aiSummary}</Typography.Text>
                      </div>
                    </Space>
                  </Typography>
                </Splitter.Panel>
              </Splitter>
            </Card>
          </>
        )}
      </Modal>

      <Modal
        width={1200}
        open={showCompare}
        title="文档比对"
        onCancel={handleCloseCompare}
        maskClosable={true}
        closable={true}
        footer={
          <Flex align="center" justify="space-between" style={{ width: '100%' }}>
            <Typography.Text type={similarityScore !== undefined ? undefined : 'secondary'}>
              {similarityScore !== undefined
                ? `语义相似度：${similarityScore.toFixed(4)}`
                : '点击按钮计算语义相似度 -->'}
            </Typography.Text>
            <Space>
              <Button
                type="primary"
                onClick={handleSimilarityCompare}
                loading={similarityLoading}
                disabled={!compareDoc1 || !compareDoc2}
              >
                语义相似度
              </Button>
            </Space>
          </Flex>
        }
        forceRender={true}
      >
        {compareDoc1 && compareDoc2 && (
          <Splitter style={{ height: 700 }}>
            <Splitter.Panel collapsible>
              <ProDescriptions<Document>
                column={2}
                title={`ID: ${compareDoc1.id}`}
                dataSource={compareDoc1}
                columns={columns as ProDescriptionsItemProps<Document>[]}
                style={{ marginRight: 20 }}
                extra={renderMenus(compareDoc1)}
              />
              <Card
                className={styles.contentCard}
                title={
                  <>
                    <OpenAIOutlined /> 文章内容
                  </>
                }
                style={{ marginTop: 20, padding: 0, marginRight: 20 }}
              >
                <Typography
                  style={{
                    margin: 2,
                    padding: '16px 20px',
                    backgroundColor: token.colorFillAlter,
                    borderRadius: 8,
                    maxHeight: 300,
                    overflow: 'auto',
                  }}
                >
                  <Typography.Title level={4} style={{ marginBottom: 14 }}>
                    <Tooltip title={compareDoc1.title}>{compareDoc1.title}</Tooltip>
                  </Typography.Title>
                  <Typography.Text>
                    {renderText(compareDoc1.content, compareDoc1.format)}
                  </Typography.Text>
                </Typography>
              </Card>
            </Splitter.Panel>
            <Splitter.Panel collapsible>
              <ProDescriptions<Document>
                column={2}
                title={`ID: ${compareDoc2.id}`}
                dataSource={compareDoc2}
                style={{ marginLeft: 20 }}
                columns={columns as ProDescriptionsItemProps<Document>[]}
                extra={renderMenus(compareDoc2)}
              />
              <Card
                className={styles.contentCard}
                title={
                  <>
                    <OpenAIOutlined /> 文章内容
                  </>
                }
                style={{ marginTop: 20, padding: 0, marginLeft: 20, marginRight: 20 }}
              >
                <Typography
                  style={{
                    margin: 2,
                    padding: '16px 20px',
                    backgroundColor: token.colorFillAlter,
                    borderRadius: 8,
                    maxHeight: 300,
                    overflow: 'auto',
                  }}
                >
                  <Typography.Title level={4} style={{ marginBottom: 14 }}>
                    <Tooltip title={compareDoc2.title}>{compareDoc2.title}</Tooltip>
                  </Typography.Title>
                  <Typography.Text>
                    {renderText(compareDoc2.content, compareDoc2.format)}
                  </Typography.Text>
                </Typography>
              </Card>
            </Splitter.Panel>
          </Splitter>
        )}
      </Modal>
    </PageContainer>
  );
};

export default DocumentList;

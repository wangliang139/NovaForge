import { CalendarOutlined } from '@ant-design/icons';
import { PageContainer } from '@ant-design/pro-components';
import {
  Calendar as AntdCalendar,
  Badge,
  Button,
  Card,
  DatePicker,
  Flex,
  Rate,
  Row,
  Space,
  Switch,
  Table,
  TableProps,
  Tag,
  theme,
  Typography,
} from 'antd';
import dayjs, { Dayjs } from 'dayjs';
import utc from 'dayjs/plugin/utc';
import React, { useState } from 'react';
import { queryCalendars } from '@/services/gateway/document';
import { CalendarItem, CalendarSource, CalendarType } from '@/services/gateway/document';

dayjs.extend(utc);

type ColumnsType<T extends object> = TableProps<T>['columns'];

const getImportanceColor = (importance: number) => {
  if (importance >= 3) return 'red';
  if (importance === 2) return 'orange';
  if (importance === 1) return 'blue';
  return 'default';
};

const CalendarPage: React.FC = () => {
  const { token } = theme.useToken();
  const [date, setDate] = useState<Dayjs>(dayjs());
  const [type, setType] = useState<CalendarType>(CalendarType.UNSPECIFIED);
  const [minImportance, setMinImportance] = useState<number>(1);
  const [loading, setLoading] = useState(false);
  const [events, setEvents] = useState<CalendarItem[]>([]);
  const [showDetail, setShowDetail] = useState(false);

  const loadData = async (date: Dayjs, t?: CalendarType) => {
    setLoading(true);
    try {
      const list = await queryCalendars({
        date: date.toDate(),
        type: (t ?? type) as any,
        minImportance,
      });
      setEvents(list || []);
    } finally {
      setLoading(false);
    }
  };

  React.useEffect(() => {
    loadData(date, type);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [date, type, minImportance]);

  const dateCellRender = (current: Dayjs) => {
    const dId = parseInt(current.format('YYYYMMDD'), 10);
    const dayEvents = events.filter((e) => e.dateId === dId);
    if (!dayEvents.length) return null;
    return (
      <ul style={{ listStyle: 'none', padding: 0, margin: 0 }}>
        {dayEvents.slice(0, 3).map((item) => (
          <li
            key={item.id}
            style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}
          >
            <Badge color={getImportanceColor(item.importance)} text={item.title} />
          </li>
        ))}
        {dayEvents.length > 3 ? <li>…</li> : null}
      </ul>
    );
  };

  const formatSource = (source: CalendarSource) => {
    switch (source) {
      case CalendarSource.GATEIO:
        return 'Gate';
      case CalendarSource.JIN10:
        return '金十数据';
      default:
        return '-';
    }
  };

  const formatType = (type: CalendarType) => {
    switch (type) {
      case CalendarType.ECONOMIC_DATA:
        return '经济数据';
      case CalendarType.PROJECT_EVENT:
        return '项目事件';
      case CalendarType.TOKEN_UNLOCK:
        return '代币解锁';
      case CalendarType.SUMMIT_EVENT:
        return '峰会活动';
      case CalendarType.FINANCING:
        return '融资';
      case CalendarType.OTHER:
        return '其他';
      case CalendarType.EVENTS:
        return '重要事件';
      default:
        return '-';
    }
  };

  const formatCategory = (category: string) => {
    switch (category) {
      case 'NewProductRelease':
        return '新品发布';
      case 'TokenUnlock':
        return '代币解锁';
      case 'SummitEvents':
        return '峰会活动';
      case 'Financing':
        return '融资';
      case 'Other':
        return '其他';
      default:
        return category;
    }
  };

  const getImportanceRateColor = (row: CalendarItem) => {
    if (row.importance <= 2) return token.colorWarning;
    return token.colorError;
  };

  const getFontColor = (row: CalendarItem) => {
    if (row.importance <= 2) return token.colorText;
    return token.colorError;
  };

  const columns: ColumnsType<CalendarItem> = [
    {
      title: '时间（UTC）',
      dataIndex: 'publishedAt',
      width: 120,
      ellipsis: true,
      render: (text: number, row: CalendarItem) => {
        return (
          <span style={{ color: getFontColor(row) }}>
            {text >= 0 ? dayjs.unix(text).utc().format('HH:mm') : ''}
          </span>
        );
      },
    },
    {
      title: '来源',
      dataIndex: 'source',
      width: 120,
      ellipsis: true,
      render: (text: CalendarSource, row: CalendarItem) => {
        return <span style={{ color: getFontColor(row) }}>{formatSource(text)}</span>;
      },
    },
    {
      title: '类型',
      dataIndex: 'type',
      hidden: type !== CalendarType.UNSPECIFIED,
      width: 120,
      ellipsis: true,
      render: (text: CalendarType, row: CalendarItem) => {
        return <span style={{ color: getFontColor(row) }}>{formatType(text)}</span>;
      },
    },
    {
      title: 'Category',
      dataIndex: 'category',
      hidden: type !== CalendarType.UNSPECIFIED && type !== CalendarType.PROJECT_EVENT,
      ellipsis: true,
      width: 120,
      render: (text: string, row: CalendarItem) => {
        return <span style={{ color: getFontColor(row) }}>{formatCategory(text)}</span>;
      },
    },
    {
      title: '地区',
      dataIndex: 'country',
      hidden:
        type !== CalendarType.ECONOMIC_DATA &&
        type !== CalendarType.UNSPECIFIED &&
        type !== CalendarType.EVENTS,
      ellipsis: true,
      width: 120,
      render: (dom, row) => {
        return <span style={{ color: getFontColor(row) }}>{dom}</span>;
      },
    },
    {
      title: '重要度',
      dataIndex: 'importance',
      ellipsis: true,
      align: 'center',
      width: 130,
      render: (v: number, row: CalendarItem) => (
        <Rate
          value={v}
          count={5}
          style={{
            fontSize: 12,
            color: getImportanceRateColor(row),
          }}
          disabled
        />
      ),
    },
    {
      title: '事件',
      dataIndex: 'title',
      render: (dom, row) => {
        return (
          <>
            <Row style={{ color: getFontColor(row) }}>{dom}</Row>
            <Row hidden={!showDetail}>{row.content}</Row>
          </>
        );
      },
    },
    {
      title: '前值',
      width: 100,
      hidden: type !== CalendarType.ECONOMIC_DATA,
      render: (dom, row) => {
        return row.ext?.previous ? (
          <Tag style={{ color: getFontColor(row) }}>{row.ext?.previous + row.ext?.unit}</Tag>
        ) : (
          '--'
        );
      },
    },
    {
      title: '预期',
      width: 100,
      hidden: type !== CalendarType.ECONOMIC_DATA,
      render: (dom, row) => {
        return row.ext?.consensus ? (
          <Tag style={{ color: getFontColor(row) }}>{row.ext?.consensus + row.ext?.unit}</Tag>
        ) : (
          '--'
        );
      },
    },
    {
      title: '实际',
      width: 100,
      hidden: type !== CalendarType.ECONOMIC_DATA,
      render: (dom, row) => {
        return row.ext?.actual ? (
          <Tag style={{ color: getFontColor(row) }}>{row.ext?.actual + row.ext?.unit}</Tag>
        ) : (
          '--'
        );
      },
    },
    {
      title: '操作',
      width: 100,
      render: (dom, row) => {
        return [
          <Button
            type="link"
            key="view"
            disabled={!row.url}
            onClick={() => window.open(row.url, '_blank')}
          >
            查看
          </Button>,
        ];
      },
    },
  ];

  const tabList = [
    { label: '全部', key: CalendarType.UNSPECIFIED },
    { label: '经济数据', key: CalendarType.ECONOMIC_DATA },
    { label: '重要事件', key: CalendarType.EVENTS },
    { label: '项目事件', key: CalendarType.PROJECT_EVENT },
    { label: '代币解锁', key: CalendarType.TOKEN_UNLOCK },
    { label: '峰会活动', key: CalendarType.SUMMIT_EVENT },
    { label: '融资', key: CalendarType.FINANCING },
    { label: '其他', key: CalendarType.OTHER },
  ];
  return (
    <PageContainer>
      <Row gutter={[16, 16]}>
        <Card loading={loading} title="选择日期" hidden styles={{ body: { padding: 12 } }}>
          <AntdCalendar
            value={date}
            onSelect={(v) => setDate(v)}
            onPanelChange={(v) => setDate(v)}
            cellRender={dateCellRender}
          />
        </Card>
        <Card
          loading={loading}
          tabList={tabList}
          activeTabKey={type}
          onTabChange={(v) => setType(v as CalendarType)}
          styles={{ body: { paddingTop: 12 } }}
          tabBarExtraContent={
            <Flex gap="middle" justify={'space-between'} align={'center'}>
              <Space>
                <Typography.Text strong={false}>展示详情</Typography.Text>
                <Switch
                  checked={showDetail}
                  size="small"
                  onChange={() => setShowDetail(!showDetail)}
                />
              </Space>
              <Space>
                <Typography.Text>只看重要</Typography.Text>
                <Switch
                  checked={minImportance >= 3}
                  size="small"
                  onChange={() => setMinImportance(minImportance >= 3 ? 1 : 3)}
                />
              </Space>
              <Space>
                <CalendarOutlined style={{ fontSize: 18 }} />
                <DatePicker
                  allowClear={false}
                  defaultValue={date}
                  value={date}
                  onChange={(v) => setDate(v)}
                />
              </Space>
            </Flex>
          }
        >
          <Table<CalendarItem>
            columns={columns}
            pagination={false}
            dataSource={events}
            // expandable={{
            //   expandedRowRender: (record) => <p style={{ margin: 0 }}>{record.content}</p>,
            //   rowExpandable: (record) => record?.content !== '',
            // }}
          />
        </Card>
      </Row>
    </PageContainer>
  );
};

export default CalendarPage;

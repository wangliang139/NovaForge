import { Avatar } from 'antd';
import { LlmMessage, LlmMessageRole, LlmMessageRoleColor } from '../types';

export const getRoleAvatar = (row: LlmMessage) => {
  const role = row.role as LlmMessageRole;
  let name = row.name || '';

  if (!name) {
    name =
      role === LlmMessageRole.USER
        ? 'User'
        : role === LlmMessageRole.ASSISTANT
        ? 'Asst'
        : role === LlmMessageRole.SYSTEM
        ? 'Sys'
        : 'Tool';
  }
  return (
    <Avatar
      style={{
        backgroundColor: LlmMessageRoleColor[role as LlmMessageRole],
        color: '#000000A6',
        fontWeight: 'bold',
        fontSize: 8,
      }}
      alt={role || ''}
    >
      {name.length > 5 ? name.slice(0, 5) : name}
    </Avatar>
  );
};

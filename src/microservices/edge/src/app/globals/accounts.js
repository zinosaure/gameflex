

export const isAdministrator = (roleLevel) => Number(roleLevel) <= 1;

export const atUsername = (username) => `@${username}`;

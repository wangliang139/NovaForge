export function enumToOptions<T extends object>(
  enumObj: T,
  ...keys: string[]
): {
  label: string;
  value: any;
}[] {
  const options = Object.keys(enumObj)
    .filter((key) => {
      return isNaN(Number(key)) && keys.indexOf(key) === -1;
    })
    .map((key) => ({
      label: key,
      value: enumObj[key as keyof T],
    }));
  return options;
}

export function getEnumKeyByValue<T>(enumObj: T, value: T[keyof T]): keyof T | undefined {
  for (const key in enumObj) {
    if (enumObj[key] === value) {
      return key as keyof T;
    }
  }
  return undefined;
}

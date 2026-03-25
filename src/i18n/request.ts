import { getRequestConfig } from 'next-intl/server';

// Disable locale prefix routing for now
export default getRequestConfig(async () => {
  // Use a default locale without prefix
  const locale = 'en';
  
  return {
    locale,
    messages: (await import(`../messages/${locale}.json`)).default,
  };
});
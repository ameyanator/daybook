import type { ConfigContext, ExpoConfig } from 'expo/config';

// Static settings live in app.json. This file adds the Google Sign-In config
// plugin only when its iOS URL scheme is provided, so the project still runs
// before Google OAuth clients exist.
//
// GOOGLE_IOS_URL_SCHEME is the iOS OAuth client ID reversed, e.g.
// com.googleusercontent.apps.1234567890-abcdef
export default ({ config }: ConfigContext): ExpoConfig => {
  const iosUrlScheme = process.env.GOOGLE_IOS_URL_SCHEME;
  return {
    ...(config as ExpoConfig),
    plugins: [
      ...(config.plugins ?? []),
      ...(iosUrlScheme
        ? [['@react-native-google-signin/google-signin', { iosUrlScheme }] as [string, object]]
        : []),
    ],
  };
};

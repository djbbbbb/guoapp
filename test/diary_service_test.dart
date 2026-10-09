import 'package:duanju_app/diary_service.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  setUp(DiaryService.clear);

  test('preserves network errors while removing URL credentials and paths', () {
    final message = DiaryService.sanitize(
      'HTTP error 403 for '
      'https://user:password@media.test:8443/private/playlist.m3u8?token=secret '
      'and http://127.0.0.1:49349/session-id/segment.m4s',
    );
    expect(message, contains('HTTP error 403'));
    expect(message, contains('https://media.test:8443/[redacted]'));
    expect(message, contains('http://127.0.0.1:49349/[redacted]'));
    for (final secret in [
      'user:password',
      '/private/',
      'playlist.m3u8',
      'token=secret',
      'session-id',
      'segment.m4s',
    ]) {
      expect(message, isNot(contains(secret)));
    }
  });

  test('removes nested decoder keys and identity headers', () {
    final message = DiaryService.sanitize(
      'seg_format_options=decryption_key=synthetic-cenc-secret, status=failed\n'
      'Authorization: Bearer synthetic-authorization-token\n'
      'Cookie: first=secret-one; second=secret-two\n'
      'X-Api-Key="synthetic-api-secret"\n'
      '"decryption_key": "quoted-synthetic-key"\n'
      'password=synthetic-password',
    );
    expect(message, contains('seg_format_options=decryption_key=[redacted]'));
    expect(message, contains('status=failed'));
    for (final secret in [
      'synthetic-cenc-secret',
      'synthetic-authorization-token',
      'secret-one',
      'secret-two',
      'synthetic-api-secret',
      'quoted-synthetic-key',
      'synthetic-password',
    ]) {
      expect(message, isNot(contains(secret)));
    }
  });

  test('redacts bare keys and explicit sensitive values before storing', () {
    const key = '00112233445566778899aabbccddeeff';
    DiaryService.add(
      'decoder failed: $key; credential synthetic-private-value',
      sensitiveValues: const ['synthetic-private-value', ''],
    );
    expect(DiaryService.fullText, contains('decoder failed: [redacted]'));
    expect(DiaryService.fullText, isNot(contains(key)));
    expect(DiaryService.fullText, isNot(contains('synthetic-private-value')));
  });

  test('removes local file URL paths', () {
    final message = DiaryService.sanitize(
      'Could not open file:///private/user/downloads/synthetic-video.mp4',
    );
    expect(message, contains('Could not open'));
    expect(message, isNot(contains('/private/user/')));
    expect(message, isNot(contains('synthetic-video.mp4')));
  });
}

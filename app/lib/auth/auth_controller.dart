import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../api/auth_api.dart';

enum AuthStatus { unknown, signedOut, signedIn }

class AuthController extends ChangeNotifier {
  AuthController({AuthApi? api, FlutterSecureStorage? storage})
    : _api = api ?? AuthApi(),
      _storage = storage ?? const FlutterSecureStorage();

  static const _tokenKey = 'session_token';

  final AuthApi _api;
  final FlutterSecureStorage _storage;

  AuthStatus status = AuthStatus.unknown;
  User? user;
  String? _token;

  /// Restores a saved session, if there is one, when the app starts.
  Future<void> restore() async {
    try {
      final token = await _storage.read(key: _tokenKey);
      if (token != null) {
        user = await _api.me(token);
        _token = token;
        status = AuthStatus.signedIn;
      } else {
        status = AuthStatus.signedOut;
      }
    } on ApiException catch (e) {
      // Only discard the token if the server rejected it, not on a network error.
      if (e.isUnauthorized) await _storage.delete(key: _tokenKey);
      status = AuthStatus.signedOut;
    } catch (_) {
      status = AuthStatus.signedOut;
    }
    notifyListeners();
  }

  Future<void> login(String identifier, String password) async {
    await _finish(await _api.login(identifier: identifier, password: password));
  }

  Future<void> register(String username, String email, String password) async {
    await _finish(
      await _api.register(username: username, email: email, password: password),
    );
  }

  Future<void> logout() async {
    final token = _token;
    _token = null;
    user = null;
    status = AuthStatus.signedOut;
    notifyListeners();
    await _storage.delete(key: _tokenKey);
    if (token != null) {
      try {
        await _api.logout(token);
      } catch (_) {}
    }
  }

  Future<void> _finish(AuthResult result) async {
    await _storage.write(key: _tokenKey, value: result.token);
    _token = result.token;
    user = result.user;
    status = AuthStatus.signedIn;
    notifyListeners();
  }
}

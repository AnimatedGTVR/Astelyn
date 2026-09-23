import 'package:flutter_test/flutter_test.dart';

import 'package:astelyn/auth/auth_controller.dart';
import 'package:astelyn/main.dart';

void main() {
  testWidgets('shows the login screen when signed out', (tester) async {
    final auth = AuthController()..status = AuthStatus.signedOut;

    await tester.pumpWidget(AstelynApp(auth: auth));

    expect(find.text('Welcome back'), findsOneWidget);
    expect(find.text('Log in'), findsOneWidget);

    await tester.tap(find.text('Need an account? Sign up'));
    await tester.pump();

    expect(find.text('Create your account'), findsOneWidget);
    expect(find.text('Email'), findsOneWidget);
  });
}

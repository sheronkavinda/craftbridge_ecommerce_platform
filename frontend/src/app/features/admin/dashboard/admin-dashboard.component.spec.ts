import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import {
  AuditLog,
  RefundRecord,
  RevenueSummary,
  SellerApplication,
  UserSummary,
} from '../admin.models';
import { AdminService } from '../admin.service';
import { AdminDashboardComponent } from './admin-dashboard.component';

describe('AdminDashboardComponent', () => {
  let component: AdminDashboardComponent;
  let fixture: ComponentFixture<AdminDashboardComponent>;
  let mockAdminService: {
    getRevenueSummary: jasmine.Spy;
    listSellerApplications: jasmine.Spy;
    listRefunds: jasmine.Spy;
    listUsers: jasmine.Spy;
    listAuditLogs: jasmine.Spy;
  };

  const sampleRevenue: RevenueSummary = {
    gross_verified_payments: 29200,
    total_refunds: 500,
    net_revenue: 28700,
    verified_payment_count: 4,
    refund_count: 1,
    generated_at: '2026-11-14T08:00:00Z',
  };

  const sampleSellers: SellerApplication[] = [
    {
      id: 'seller-001',
      user_id: 'seller-u1',
      store_name: 'Sunil Wood Crafts',
      status: 'APPROVED',
      notes: '',
      submitted_at: '2026-11-10T10:00:00Z',
    },
    {
      id: 'seller-002',
      user_id: 'seller-u2',
      store_name: 'Amali Handlooms',
      status: 'PENDING',
      notes: '',
      submitted_at: '2026-11-11T10:00:00Z',
    },
  ];

  const sampleRefunds: RefundRecord[] = [
    {
      id: 'refund-001',
      order_id: 'order-001',
      amount: 2500,
      status: 'PENDING',
      reason: 'Item arrived damaged',
      created_at: '2026-11-12T10:00:00Z',
    },
    {
      id: 'refund-002',
      order_id: 'order-002',
      amount: 1400,
      status: 'PENDING',
      reason: 'Wrong size delivered',
      created_at: '2026-11-12T11:00:00Z',
    },
  ];

  const sampleUsers: UserSummary[] = [
    {
      id: 'buyer-001',
      name: 'Nimal Perera',
      email: 'nimal@craftbridge.test',
      role: 'BUYER',
      status: 'ACTIVE',
      created_at: '2026-11-01T10:00:00Z',
    },
    {
      id: 'buyer-003',
      name: 'Ruwan Fernando',
      email: 'ruwan@craftbridge.test',
      role: 'BUYER',
      status: 'SUSPENDED',
      created_at: '2026-11-02T10:00:00Z',
    },
  ];

  const sampleAuditLogs: AuditLog[] = [
    {
      id: 1,
      actor_id: 'admin-001',
      actor_name: 'System Admin',
      action: 'USER_STATUS_UPDATE',
      target: 'user:buyer-003',
      outcome: 'SUCCESS',
      request_id: 'req-001',
      details: 'status set to SUSPENDED',
      created_at: '2026-11-14T07:55:00Z',
    },
  ];

  beforeEach(async () => {
    mockAdminService = {
      getRevenueSummary: jasmine.createSpy('getRevenueSummary').and.returnValue(of(sampleRevenue)),
      listSellerApplications: jasmine
        .createSpy('listSellerApplications')
        .and.returnValue(of(sampleSellers)),
      listRefunds: jasmine.createSpy('listRefunds').and.returnValue(of(sampleRefunds)),
      listUsers: jasmine.createSpy('listUsers').and.returnValue(of(sampleUsers)),
      listAuditLogs: jasmine.createSpy('listAuditLogs').and.returnValue(of(sampleAuditLogs)),
    };

    await TestBed.configureTestingModule({
      imports: [AdminDashboardComponent],
      providers: [provideRouter([]), { provide: AdminService, useValue: mockAdminService }],
    }).compileComponents();

    fixture = TestBed.createComponent(AdminDashboardComponent);
    component = fixture.componentInstance;
  });

  it('renders KPI values and LKR currency amounts from mocked service data', () => {
    fixture.detectChanges();

    expect(component.pendingSellerCount()).toBe(1);
    expect(component.pendingRefundStats()).toEqual({ count: 2, totalAmount: 3900 });
    expect(component.suspendedUserCount()).toBe(1);

    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.textContent).toContain('LKR 29,200.00');
    expect(compiled.textContent).toContain('LKR 28,700.00');
    expect(compiled.textContent).toContain('LKR 3,900.00');
    expect(compiled.textContent).toContain('User status update');
  });

  it('keeps other sections visible when one section fails to load', () => {
    mockAdminService.getRevenueSummary.and.returnValue(
      throwError(() => ({ error: { error: 'Revenue service unavailable' } })),
    );

    fixture.detectChanges();

    expect(component.revenueError()).toBe('Revenue service unavailable');
    expect(component.pendingSellerCount()).toBe(1);
    expect(component.pendingRefundStats().count).toBe(2);
    expect(component.recentAuditLogs().length).toBe(1);

    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.textContent).toContain('Revenue service unavailable');
    expect(compiled.textContent).toContain('LKR 3,900.00');
    expect(compiled.textContent).toContain('User status update');
  });
});


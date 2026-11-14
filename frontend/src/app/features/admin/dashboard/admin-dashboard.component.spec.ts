import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { DashboardSummary } from '../admin.models';
import { AdminService } from '../admin.service';
import { AdminDashboardComponent } from './admin-dashboard.component';

describe('AdminDashboardComponent', () => {
  let component: AdminDashboardComponent;
  let fixture: ComponentFixture<AdminDashboardComponent>;
  let mockAdminService: {
    getDashboardSummary: jasmine.Spy;
  };

  const sampleDashboard: DashboardSummary = {
    users: {
      total: 12,
      active: 11,
      suspended: 1,
      by_role: {
        BUYER: 8,
        SELLER: 3,
        ADMIN: 1,
      },
    },
    sellers: {
      pending: 1,
      approved: 2,
      rejected: 0,
      suspended: 0,
    },
    refunds: {
      pending_count: 2,
      pending_amount: 3900,
    },
    categories: {
      total: 6,
      active: 5,
    },
    revenue: {
      gross_verified_payments: 29200,
      total_refunds: 500,
      net_revenue: 28700,
      verified_payment_count: 4,
      refund_count: 1,
      generated_at: '2026-11-14T08:00:00Z',
    },
    recent_activity: [
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
    ],
    generated_at: '2026-11-14T08:00:00Z',
  };

  beforeEach(async () => {
    mockAdminService = {
      getDashboardSummary: jasmine
        .createSpy('getDashboardSummary')
        .and.returnValue(of(sampleDashboard)),
    };

    await TestBed.configureTestingModule({
      imports: [AdminDashboardComponent],
      providers: [provideRouter([]), { provide: AdminService, useValue: mockAdminService }],
    }).compileComponents();

    fixture = TestBed.createComponent(AdminDashboardComponent);
    component = fixture.componentInstance;
  });

  it('renders KPI values, LKR currency amounts, role breakdown, and categories from getDashboardSummary', () => {
    fixture.detectChanges();

    expect(mockAdminService.getDashboardSummary).toHaveBeenCalledTimes(1);
    expect(component.pendingSellerCount()).toBe(1);
    expect(component.pendingRefundStats()).toEqual({ count: 2, totalAmount: 3900 });
    expect(component.suspendedUserCount()).toBe(1);
    expect(component.userStats().by_role).toEqual({ BUYER: 8, SELLER: 3, ADMIN: 1 });
    expect(component.categoryStats()).toEqual({ total: 6, active: 5 });

    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.textContent).toContain('LKR 29,200.00');
    expect(compiled.textContent).toContain('LKR 28,700.00');
    expect(compiled.textContent).toContain('LKR 3,900.00');
    expect(compiled.textContent).toContain('Users by role');
    expect(compiled.textContent).toContain('Categories');
    expect(compiled.textContent).toContain('User status update');
  });

  it('shows error state with Retry when the request fails', () => {
    mockAdminService.getDashboardSummary.and.returnValue(
      throwError(() => ({ error: { error: 'Dashboard summary unavailable' } })),
    );

    fixture.detectChanges();

    expect(component.loadError()).toBe('Dashboard summary unavailable');

    const compiled = fixture.nativeElement as HTMLElement;
    const alertEl = compiled.querySelector('[role="alert"]');
    expect(alertEl?.textContent).toContain('Dashboard summary unavailable');
    expect(alertEl?.textContent).toContain('Retry');
  });
});

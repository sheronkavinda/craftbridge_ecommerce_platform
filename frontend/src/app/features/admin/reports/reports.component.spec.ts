import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import {
  DailyRevenuePoint,
  RevenueSummary,
  SellerSalesReport,
} from '../admin.models';
import { AdminService } from '../admin.service';
import { ReportsComponent } from './reports.component';

describe('ReportsComponent', () => {
  let component: ReportsComponent;
  let fixture: ComponentFixture<ReportsComponent>;
  let mockAdminService: {
    getRevenueSummary: jasmine.Spy;
    getDailyRevenue: jasmine.Spy;
    getSellerRevenue: jasmine.Spy;
  };

  const sampleSummary: RevenueSummary = {
    gross_verified_payments: 29200,
    total_refunds: 500,
    net_revenue: 28700,
    verified_payment_count: 4,
    refund_count: 1,
    generated_at: '2026-11-14T08:00:00Z',
  };

  const sampleDaily: DailyRevenuePoint[] = [
    {
      date: '2026-11-13',
      verified_payments: 8300,
      refunds: 500,
      net_revenue: 7800,
    },
    {
      date: '2026-11-11',
      verified_payments: 12500,
      refunds: 0,
      net_revenue: 12500,
    },
  ];

  const sampleSellers: SellerSalesReport[] = [
    {
      seller_id: 'seller-001',
      store_name: 'Sunil Wood Crafts',
      order_count: 3,
      verified_payments: 26100,
      refunds: 500,
      net_revenue: 25600,
    },
    {
      seller_id: 'seller-002',
      store_name: 'Amali Handlooms',
      order_count: 1,
      verified_payments: 3100,
      refunds: 0,
      net_revenue: 3100,
    },
  ];

  beforeEach(async () => {
    mockAdminService = {
      getRevenueSummary: jasmine.createSpy('getRevenueSummary').and.returnValue(of(sampleSummary)),
      getDailyRevenue: jasmine.createSpy('getDailyRevenue').and.returnValue(of(sampleDaily)),
      getSellerRevenue: jasmine.createSpy('getSellerRevenue').and.returnValue(of(sampleSellers)),
    };

    await TestBed.configureTestingModule({
      imports: [ReportsComponent],
      providers: [{ provide: AdminService, useValue: mockAdminService }],
    }).compileComponents();

    fixture = TestBed.createComponent(ReportsComponent);
    component = fixture.componentInstance;
  });

  it('renders revenue summary cards, chronological daily chart bars, and seller sales totals', () => {
    fixture.detectChanges();

    expect(component.chronologicalDailyPoints().map((p) => p.date)).toEqual([
      '2026-11-11',
      '2026-11-13',
    ]);
    expect(component.sellerTotals()).toEqual({
      orderCount: 4,
      verifiedPayments: 29200,
      refunds: 500,
      netRevenue: 28700,
    });

    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.textContent).toContain('LKR 29,200.00');
    expect(compiled.textContent).toContain('LKR 28,700.00');
    expect(compiled.textContent).toContain('Sunil Wood Crafts');
  });

  it('validates that From date is not after To date', () => {
    fixture.detectChanges();

    component.onFromDateChange('2026-11-15');
    component.onToDateChange('2026-11-10');
    expect(component.dateValidationError()).toContain('From date must be on or before To date');
  });

  it('isolates section failures so one failing report endpoint does not hide other sections', () => {
    mockAdminService.getDailyRevenue.and.returnValue(
      throwError(() => ({ error: { error: 'Daily breakdown unavailable' } })),
    );

    fixture.detectChanges();

    expect(component.dailyError()).toBe('Daily breakdown unavailable');
    expect(component.revenueSummary()?.net_revenue).toBe(28700);
    expect(component.sortedSellers().length).toBe(2);
  });
});
